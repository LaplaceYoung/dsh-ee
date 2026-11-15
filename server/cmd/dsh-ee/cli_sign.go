package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

// 扩展的签名（docs/plugins.md §9.3）：作者本地生成密钥、给清单（声明式连同接口文档）签一个
// Ed25519 detached 签名，安装时 dsh-ee 用受信任的公钥核对。私钥只在作者手里，dsh-ee 不生成、
// 不保存、不上传；这里只用了标准库的 crypto/ed25519。

// trustedKeysFile 是环境变量名：受信任的签名公钥（PEM）文件。命令行和 dsh-ee 服务端用同一个名字。
const trustedKeysEnv = "DSH_EE_EXTENSION_TRUSTED_KEYS"

func cliExtKeygen(args []string, stdout, stderr io.Writer) int {
	rest, vals, on, err := options(args, []string{"--public"}, []string{"--json"})
	if err != nil || len(rest) != 1 {
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		fmt.Fprintln(stderr, tr("用法：dsh-ee extension keygen <私钥文件> [--public 公钥文件] [--json]", "Usage: dsh-ee extension keygen <private-key-file> [--public public-key-file] [--json]"))
		return 2
	}
	privPath := rest[0]
	pubPath := vals["--public"]
	if pubPath == "" {
		// sign.key → sign.pub；没有扩展名就加 .pub
		ext := filepath.Ext(privPath)
		if ext == "" {
			pubPath = privPath + ".pub"
		} else {
			pubPath = strings.TrimSuffix(privPath, ext) + ".pub"
		}
	}
	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintf(stderr, tr("%s 已经存在，不覆盖。换一个文件名，或者先把它挪走\n", "%s already exists; not overwriting. Use another file name, or move it away first\n"), p)
			return 1
		}
	}
	pub, priv, err := extension.GenerateKey()
	if err != nil {
		fmt.Fprintln(stderr, tr("生成密钥失败：", "key generation failed: "), err)
		return 1
	}
	privPEM, err := extension.MarshalPrivateKeyPEM(priv)
	if err != nil {
		fmt.Fprintln(stderr, tr("生成密钥失败：", "key generation failed: "), err)
		return 1
	}
	pubPEM, err := extension.MarshalPublicKeyPEM(pub)
	if err != nil {
		fmt.Fprintln(stderr, tr("生成密钥失败：", "key generation failed: "), err)
		return 1
	}
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		fmt.Fprintln(stderr, tr("写私钥：", "write private key: "), err)
		return 1
	}
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		fmt.Fprintln(stderr, tr("写公钥：", "write public key: "), err)
		return 1
	}
	id := extension.KeyID(pub)
	if on["--json"] {
		printJSON(stdout, map[string]any{"private_key": privPath, "public_key": pubPath, "key_id": id, "alg": extension.SignatureAlg})
		return 0
	}
	fmt.Fprintf(stdout, tr("私钥：%s（权限 0600；不要提交到仓库，也不要发给别人）\n", "private key: %s (mode 0600; do not commit it or share it with anyone)\n"), privPath)
	fmt.Fprintf(stdout, tr("公钥：%s\n", "public key: %s\n"), pubPath)
	fmt.Fprintf(stdout, tr("key id：%s\n\n", "key id: %s\n\n"), id)
	fmt.Fprintln(stdout, tr("给扩展签名：", "Sign the extension:"))
	fmt.Fprintf(stdout, tr("  dsh-ee extension sign <扩展目录> --key %s\n\n", "  dsh-ee extension sign <extension-dir> --key %s\n\n"), privPath)
	fmt.Fprintln(stdout, tr("把公钥交给管理员，加进 dsh-ee 的受信任公钥（安装时据此核对签名）：", "Give the public key to an admin to add to the trusted keys in dsh-ee (signatures are checked against it at install time):"))
	fmt.Fprintf(stdout, "  %s=%s\n", trustedKeysEnv, pubPath)
	return 0
}

func cliExtSign(args []string, stdout, stderr io.Writer) int {
	rest, vals, on, err := options(args, []string{"--key", "-o", "--out"}, []string{"--json"})
	if err != nil || len(rest) != 1 || vals["--key"] == "" {
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		fmt.Fprintln(stderr, tr("用法：dsh-ee extension sign <目录> --key <私钥文件> [-o 文件] [--json]", "Usage: dsh-ee extension sign <dir> --key <private-key-file> [-o file] [--json]"))
		return 2
	}
	dir := rest[0]
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, tr("%s 不是一个目录\n", "%s is not a directory\n"), dir)
		return 2
	}
	res := extension.Check(os.DirFS(dir))
	if !res.OK {
		printCheck(stderr, res)
		fmt.Fprintln(stderr, tr("检查没通过，不签名。", "check failed; not signing."))
		return 1
	}
	if res.Manifest == nil {
		fmt.Fprintln(stderr, tr("读不到扩展的清单，不签名。", "cannot read the extension manifest; not signing."))
		return 1
	}
	priv, err := extension.LoadPrivateKey(vals["--key"])
	if err != nil {
		fmt.Fprintln(stderr, tr("读私钥：", "read private key: "), err)
		return 1
	}
	manifest, err := os.ReadFile(filepath.Join(dir, extension.ManifestFile))
	if err != nil {
		fmt.Fprintln(stderr, tr("读清单：", "read manifest: "), err)
		return 1
	}
	sig, raw, err := extension.SignExtension(manifest, res.OpenAPIDoc, priv)
	if err != nil {
		fmt.Fprintln(stderr, tr("签名失败：", "signing failed: "), err)
		return 1
	}
	out := vals["-o"]
	if out == "" {
		out = vals["--out"]
	}
	if out == "" {
		out = filepath.Join(dir, extension.SignatureFile)
	}
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		fmt.Fprintln(stderr, tr("写签名：", "write signature: "), err)
		return 1
	}
	if on["--json"] {
		printJSON(stdout, map[string]any{
			"file": out, "identity": sig.Identity, "digest": sig.Digest,
			"key_id": sig.KeyID, "alg": sig.Alg, "version": sig.Version,
		})
		return 0
	}
	fmt.Fprintf(stdout, tr("已签名：%s\n", "signed: %s\n"), sig.Identity)
	fmt.Fprintf(stdout, tr("签名者：%s\n", "signer: %s\n"), sig.KeyID)
	fmt.Fprintf(stdout, tr("清单和接口文档的摘要：%s\n", "manifest and OpenAPI doc digest: %s\n"), sig.Digest)
	fmt.Fprintf(stdout, tr("签名写到：%s\n\n", "signature written to: %s\n\n"), out)
	fmt.Fprintln(stdout, tr("接下来：dsh-ee extension pack <目录>，把带签名的包交给管理员；", "Next: dsh-ee extension pack <dir>, then hand the signed package to an admin;"))
	fmt.Fprintln(stdout, tr("管理员那边要有这把公钥（", "The admin side must have this public key (")+trustedKeysEnv+tr("）才算受信任。", ") for the signature to be trusted."))
	return 0
}

func cliExtVerify(args []string, stdout, stderr io.Writer) int {
	rest, vals, on, err := options(args, []string{"--key"}, []string{"--json"})
	if err != nil || len(rest) != 1 {
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		fmt.Fprintln(stderr, tr("用法：dsh-ee extension verify <目录> [--key <公钥文件>] [--json]", "Usage: dsh-ee extension verify <dir> [--key <public-key-file>] [--json]"))
		return 2
	}
	keyFile := vals["--key"]
	if keyFile == "" {
		keyFile = os.Getenv(trustedKeysEnv)
	}
	if keyFile == "" {
		fmt.Fprintln(stderr, tr("用法：dsh-ee extension verify <目录> [--key <公钥文件>] [--json]", "Usage: dsh-ee extension verify <dir> [--key <public-key-file>] [--json]"))
		fmt.Fprintln(stderr, tr("要给一把受信任的公钥来核对签名：--key <公钥文件>，或设置 ", "A trusted public key is required to verify the signature: --key <public-key-file>, or set ")+trustedKeysEnv+tr("=<公钥文件>。", "=<public-key-file>."))
		fmt.Fprintln(stderr, tr("公钥可以是 PEM 公钥、证书，或直接给签名用的私钥文件。", "The key can be a PEM public key, a certificate, or the private key file used for signing."))
		return 2
	}
	trusted, err := extension.LoadPublicKeys(keyFile)
	if err != nil {
		fmt.Fprintln(stderr, tr("读受信任的公钥：", "read trusted public key: "), err)
		return 2
	}
	dir := rest[0]
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, tr("%s 不是一个目录\n", "%s is not a directory\n"), dir)
		return 2
	}
	status := extension.VerifyDir(dir, trusted)
	if on["--json"] {
		printJSON(stdout, status)
	} else {
		switch status.Status {
		case extension.SignatureValid:
			fmt.Fprintln(stdout, tr("签名有效：", "signature valid: ")+status.Message)
		case extension.SignatureUnsigned:
			fmt.Fprintln(stderr, tr("没有签名：", "unsigned: ")+status.Message)
		default:
			fmt.Fprintln(stderr, tr("签名没通过：", "signature invalid: ")+status.Message)
		}
	}
	if status.Status != extension.SignatureValid {
		return 1
	}
	return 0
}
