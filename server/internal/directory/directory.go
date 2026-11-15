// Package directory 是企业扩展目录（dsh-ee-directory.json，format 2）的格式、签名与校验。
//
// 企业目录不是新市场，而是把「哪个版本、内容摘要、从哪来、谁声明签过名」变成一份可引用、
// 可审计的索引（docs/plugin-signing.md §2.2、roadmap-plugin-distribution.md §3）。目录本身由
// 企业用一把 Ed25519 密钥整体签名；dsh-ee 用受信任的签名公钥（trusted_signing_keys）核对目录
// 签名，再逐条检查条目。目录只负责「来源」：登记不等于安装，也不等于审核通过。
//
// 目录签名的对象是**文档里除 signature 之外的规范化 JSON**（CanonicalPayload），不是原始文件
// 字节：重新缩进、调整空白不影响验签，同一份语义只有一种签名输入。签名再绑上 directory_id
// 做域分隔，别的目录的签名挪不过来。
package directory

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// FileName 是目录文件名。目录可以放在企业静态站/对象存储，由管理员把内容贴进管理接口。
const FileName = "dsh-ee-directory.json"

const (
	// FormatVersion 是目录文档格式版本。format 2 与 dsh-index.json 的 format 1 是两份不同的
	// 文档：dsh-index 给 DSH 客户端直接消费，目录表达企业准入与来源。
	FormatVersion = 2

	// 条目类型：DSH 插件（npm 包）与服务端扩展（清单/接口文档）。
	KindPlugin    = "dsh-plugin"
	KindExtension = "extension"

	// 通道。插件里 stable 对应 dist-tag latest、beta 对应 beta；扩展的通道是目录/审核属性。
	ChannelStable = "stable"
	ChannelBeta   = "beta"

	// 来源类型：npm 包名，或固定 tarball 地址。
	SourceNPM     = "npm"
	SourceTarball = "tarball"

	// 目录签名格式：和扩展签名一样，Ed25519 detached；版本现在只有 1。
	SignatureVersion = 1
	SignatureAlg     = "ed25519"

	// 上限：目录文档最大 2 MiB、最多 500 条。条目的 tarball 大小上限按类型算（见 MaxTarballSize）。
	MaxDocumentBytes = 2 << 20
	MaxEntries       = 500
)

// 目录签名的状态。和扩展签名一样分开显示，不合并成一个「可信」。
const (
	// SignatureUnsigned 没有签名。企业目录要求整体签名，所以这不能用于 apply。
	SignatureUnsigned = "unsigned"
	// SignatureValid 签名对得上，签名公钥在受信任列表里（active 或 retired）。
	SignatureValid = "valid"
	// SignatureInvalid 有签名但格式不对、内容被改过、或者签名者不受信任。
	SignatureInvalid = "invalid"
	// SignatureRevoked 签名公钥已撤销：历史签名仍能核对，但不能用来登记目录。
	SignatureRevoked = "revoked"
)

// 条目内嵌签名的状态（只描述声明，不验证内容）。
const (
	// EntrySignatureNone 条目没带签名声明。
	EntrySignatureNone = "none"
	// EntrySignatureDeclared 条目内嵌了扩展签名声明，且 key_id 在受信任公钥里（或没配信任根时
	// 只做了结构检查）。目录里没有扩展的内容，所以这里**不能**验证签名本身，安装时再核对。
	EntrySignatureDeclared = "declared"
)

// MaxTarballSize 返回某类条目的 tarball 大小上限：插件沿用企业源的 tarball 上限（100 MiB），
// 扩展沿用 zip 包上限（8 MiB）。目录 v1 不下载 tarball，只能核对条目里声明的大小。
func MaxTarballSize(kind string) int64 {
	if kind == KindPlugin {
		return 100 << 20
	}
	return 8 << 20
}

// Document 是 dsh-ee-directory.json 的内容。
type Document struct {
	Format int `json:"format"`
	// 目录标识：一个企业可以有多份目录（如正式/测试），各自覆盖登记
	DirectoryID string         `json:"directory_id"`
	Title       extension.Text `json:"title,omitempty"`
	// 生成时间（RFC3339）；只用于展示，不参与策略
	GeneratedAt string `json:"generated_at,omitempty"`
	// 签发者：企业/部门名称和签名公钥的 key id
	Issuer  Issuer  `json:"issuer"`
	Entries []Entry `json:"entries"`
	// 目录整体签名；验签时把它排除在 canonical payload 外
	Signature *Signature `json:"signature,omitempty"`
}

// Issuer 是目录的签发者。KeyID 是签名公钥的 sha256 前 16 位十六进制，和扩展签名同一口径。
type Issuer struct {
	Name  string `json:"name"`
	KeyID string `json:"key_id"`
}

// Signature 是目录整体签名。
type Signature struct {
	Version int    `json:"version"`
	Alg     string `json:"alg"`
	KeyID   string `json:"key_id"`
	// 签的是哪份目录（域分隔，避免把别的目录的签名挪过来）
	DirectoryID string `json:"directory_id"`
	// canonical payload 的 sha256（十六进制）；目录内容任何一处变了都对不上
	Digest string `json:"digest"`
	// 对 SignedBytes(DirectoryID, Digest) 的 Ed25519 签名，base64
	Signature string `json:"signature"`
}

// Entry 是目录里的一条：插件或扩展的一个具体版本。
type Entry struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Channel string `json:"channel"`
	// 内容摘要：扩展是清单（声明式连同接口文档）的 sha256 十六进制；插件是 npm 的
	// sha512 SRI（sha512-<base64>）。和安装/审核时用的摘要同一口径。
	Digest string `json:"digest"`
	Source Source `json:"source"`
	// 可选：扩展条目内嵌的签名声明（和扩展目录里的 signature.json 同形状）。
	Signature *EntrySignature `json:"signature,omitempty"`
	// 可选：发布者信息，只作展示与审计，不参与权限判断
	Publisher *Publisher `json:"publisher,omitempty"`
	// 可选：给管理员看的说明
	Notes string `json:"notes,omitempty"`
	// 可选：兼容的 dsh-ee 最低版本
	MinVersion string `json:"min_version,omitempty"`
	// 可选：tarball 声明大小（字节）；目录 v1 不下载，只核对不超过上限
	Size int64 `json:"size,omitempty"`
	// 可选（format 2 additive）：允许在部署开启 directory.auto_upgrade 后自动升级这一条。
	// 本期只登记 / 校验，**不执行**任何自动升级；真正升级仍由管理员显式触发。
	// 缺省 false；写成 false 时不出现在 canonical payload 里，旧目录的签名不受影响。
	AutoUpgrade bool `json:"auto_upgrade,omitempty"`
}

// Source 是条目的来源。v1 只允许 npm 包名或固定 https tarball 地址，不做 Git 安装。
type Source struct {
	Type string `json:"type"`
	// npm：可选的 registry 地址（https）；空表示用 dsh-ee 自己的企业源
	Registry string `json:"registry,omitempty"`
	// tarball：固定地址，必须 https；受 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单/内网策略约束
	URL string `json:"url,omitempty"`
}

// EntrySignature 是扩展条目里内嵌的签名声明，形状与扩展的 signature.json 一致。
// 目录里没有扩展内容，所以只能核对结构、身份/摘要一致性和签名者是否受信任；签名本身在
// 安装 / 审核拿到内容时才会核对。
type EntrySignature struct {
	Version   int    `json:"version"`
	Alg       string `json:"alg"`
	KeyID     string `json:"key_id"`
	Identity  string `json:"identity"`
	Digest    string `json:"digest"`
	Signature string `json:"signature"`
}

// Publisher 是条目声明的发布者。目录不凭它做权限判断（权限仍看 dsh-ee 账号）。
type Publisher struct {
	UserID     int64  `json:"user_id,omitempty"`
	Username   string `json:"username,omitempty"`
	Department string `json:"department,omitempty"`
}

// VerifyResult 是目录整体签名的核对结果。
type VerifyResult struct {
	// unsigned / valid / invalid / revoked
	Status string `json:"status"`
	// 签名里写的 key id（有的话）
	KeyID string `json:"key_id,omitempty"`
	// 签名公钥的状态：active / retired / revoked；签名有效时才有
	KeyState string `json:"key_state,omitempty"`
	// canonical payload 的 sha256
	Digest string `json:"digest,omitempty"`
	// 给人看的一句话
	Message string `json:"message,omitempty"`
}

// Parse 读取并严格解析目录文档：字段必须都是认识的、JSON 后面不能还有别的内容、
// 文档不超过大小上限。格式/字段的语义校验在 Validate 里。
func Parse(raw []byte) (*Document, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("目录是空的")
	}
	if len(raw) > MaxDocumentBytes {
		return nil, fmt.Errorf("目录太大：%d 字节，上限 %d 字节", len(raw), MaxDocumentBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("目录不是合法的 JSON：%w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("目录 JSON 后面还有多余的内容")
	}
	return &doc, nil
}

// CanonicalPayload 是签名覆盖的字节：文档里除 signature 之外的规范化 JSON。
// 结构体字段顺序和 json.Marshal 的 map 排序保证同一份语义只有一种字节表示。
func CanonicalPayload(doc *Document) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("目录文档是空的")
	}
	c := *doc
	c.Signature = nil
	raw, err := json.Marshal(&c)
	if err != nil {
		return nil, fmt.Errorf("目录没法序列化：%w", err)
	}
	return raw, nil
}

// PayloadDigest 是 canonical payload 的 sha256 十六进制。
func PayloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// SignedBytes 是真正被 Ed25519 签的字节：<directory_id>:<payload digest>，加前缀做域分隔。
func SignedBytes(directoryID, digest string) []byte {
	return []byte("dsh-ee-directory:" + directoryID + ":" + digest)
}

// SignDocument 给目录签名，写回 doc.Signature。私钥只在调用方手里（CLI 或企业自己的运维脚本），
// dsh-ee 服务端不生成、不托管、不入库。
//
// 调用前应先 Validate 文档；这里只做签名需要的最低检查。issuer.key_id 为空时自动填上，
// 和私钥对不上时拒绝签名。
func SignDocument(doc *Document, priv ed25519.PrivateKey) error {
	if doc == nil {
		return errors.New("目录文档是空的")
	}
	if len(priv) != ed25519.PrivateKeySize {
		return errors.New("私钥不对：要一个 Ed25519 私钥")
	}
	if doc.Format != FormatVersion {
		return fmt.Errorf("只支持 format %d", FormatVersion)
	}
	if strings.TrimSpace(doc.DirectoryID) == "" {
		return errors.New("目录缺少 directory_id")
	}
	pub := priv.Public().(ed25519.PublicKey)
	keyID := extension.KeyID(pub)
	if doc.Issuer.KeyID == "" {
		doc.Issuer.KeyID = keyID
	}
	if doc.Issuer.KeyID != keyID {
		return fmt.Errorf("issuer.key_id 是 %s，和私钥（%s）不是同一把", doc.Issuer.KeyID, keyID)
	}
	payload, err := CanonicalPayload(doc)
	if err != nil {
		return err
	}
	digest := PayloadDigest(payload)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignedBytes(doc.DirectoryID, digest)))
	doc.Signature = &Signature{
		Version:     SignatureVersion,
		Alg:         SignatureAlg,
		KeyID:       keyID,
		DirectoryID: doc.DirectoryID,
		Digest:      digest,
		Signature:   sig,
	}
	return nil
}

// VerifyDocument 用受信任的公钥核对目录整体的签名。签名公钥的状态由调用方传进来（active /
// retired / revoked，和扩展签名共用 trusted_signing_keys 的生命周期）。
//
// 只做密码学校验，不判断策略：revoked 的签名仍然「对得上」，但调用方不能拿它登记目录。
func VerifyDocument(doc *Document, keys []extension.TrustedKey) *VerifyResult {
	if doc == nil {
		return &VerifyResult{Status: SignatureInvalid, Message: "目录文档是空的"}
	}
	payload, err := CanonicalPayload(doc)
	if err != nil {
		return &VerifyResult{Status: SignatureInvalid, Message: err.Error()}
	}
	digest := PayloadDigest(payload)
	res := &VerifyResult{Digest: digest}
	sig := doc.Signature
	if sig == nil {
		res.Status = SignatureUnsigned
		res.Message = "目录没有签名：企业目录必须整体签名，不能用来登记"
		return res
	}
	res.KeyID = sig.KeyID
	invalid := func(format string, args ...any) *VerifyResult {
		res.Status = SignatureInvalid
		res.Message = fmt.Sprintf(format, args...)
		return res
	}
	switch {
	case sig.Version != SignatureVersion:
		return invalid("不认识的目录签名版本 %d：dsh-ee 现在只认 %d", sig.Version, SignatureVersion)
	case sig.Alg != SignatureAlg:
		return invalid("不认识的签名算法「%s」：dsh-ee 只认 %s", sig.Alg, SignatureAlg)
	case sig.KeyID == "" || sig.DirectoryID == "" || sig.Digest == "" || sig.Signature == "":
		return invalid("签名缺少 key_id、directory_id、digest 或 signature 字段")
	case sig.KeyID != doc.Issuer.KeyID:
		return invalid("issuer.key_id 是 %s，签名的是 %s：目录头和签名必须一致", doc.Issuer.KeyID, sig.KeyID)
	case sig.DirectoryID != doc.DirectoryID:
		return invalid("签名是给目录 %s 的，这份是 %s", sig.DirectoryID, doc.DirectoryID)
	case sig.Digest != digest:
		return invalid("签名对不上目录内容：内容可能被改过")
	}
	raw, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return invalid("签名不是合法的 base64 或长度不对")
	}
	byID := map[string]extension.TrustedKey{}
	for _, k := range keys {
		if len(k.Public) != ed25519.PublicKeySize {
			continue
		}
		id := extension.KeyID(k.Public)
		if _, dup := byID[id]; !dup {
			byID[id] = k
		}
	}
	key, ok := byID[sig.KeyID]
	if !ok {
		if len(byID) == 0 {
			return invalid("签名者 %s 不受信任：还没有配置受信任的目录签名公钥", sig.KeyID)
		}
		return invalid("签名者 %s 不受信任：不在受信任的签名公钥里", sig.KeyID)
	}
	if !ed25519.Verify(key.Public, SignedBytes(sig.DirectoryID, sig.Digest), raw) {
		return invalid("签名和内容对不上：可能被改过")
	}
	state := key.State
	if state == "" {
		state = extension.KeyStateActive
	}
	res.KeyState = state
	switch state {
	case extension.KeyStateRevoked:
		res.Status = SignatureRevoked
		res.Message = "目录签名者的公钥 " + sig.KeyID + " 已撤销：历史签名仍能核对，但不能用它登记目录"
	case extension.KeyStateRetired:
		res.Status = SignatureValid
		res.Message = "签名者 " + sig.KeyID + " 的公钥已换下，仍认这个签名；新签目录请换用新公钥"
	default:
		res.Status = SignatureValid
		res.Message = "签名者 " + sig.KeyID + " 在受信任的公钥里"
	}
	return res
}

// GeneratedAtProblem 检查 generated_at 是不是 RFC3339；空串表示没写，可以接受。
func GeneratedAtProblem(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, raw); err != nil {
		return "generated_at 要写成 RFC3339，例如 2026-11-13T08:00:00Z"
	}
	return ""
}
