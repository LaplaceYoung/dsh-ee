package registry

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Blobs 是按内容寻址的 tarball 存储：文件名就是 sha512，同一个包被多个名字引用也只存一份。
// 写入先落临时文件再改名，进程中途退出不会留下半个文件。
type Blobs struct {
	dir string
}

var blobKey = regexp.MustCompile(`^[0-9a-f]{128}$`)

func NewBlobs(dataDir string) (*Blobs, error) {
	dir := filepath.Join(dataDir, "blobs")
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录 %s: %w", dir, err)
	}
	return &Blobs{dir: dir}, nil
}

// Digest 是一个 tarball 的摘要：npm 的 integrity（sha512）与旧客户端用的 shasum（sha1）。
type Digest struct {
	Key       string // sha512 hex，也是存储文件名
	Integrity string // "sha512-<base64>"
	Shasum    string // sha1 hex
	Size      int64
}

var ErrTooLarge = errors.New("Tarball exceeds the size limit")

// Put 写入并返回摘要；超过 limit 字节时返回 ErrTooLarge。
func (b *Blobs) Put(r io.Reader, limit int64) (Digest, error) {
	tmp, err := os.CreateTemp(filepath.Join(b.dir, "tmp"), "upload-*")
	if err != nil {
		return Digest{}, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h512, h1 := sha512.New(), sha1.New()
	n, err := io.Copy(io.MultiWriter(tmp, h512, h1), io.LimitReader(r, limit+1))
	if err != nil {
		return Digest{}, err
	}
	if n > limit {
		return Digest{}, ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		return Digest{}, err
	}
	sum := h512.Sum(nil)
	d := Digest{
		Key:       hex.EncodeToString(sum),
		Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum),
		Shasum:    hex.EncodeToString(h1.Sum(nil)),
		Size:      n,
	}
	dst := b.path(d.Key)
	if _, err := os.Stat(dst); err == nil {
		return d, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return Digest{}, err
	}
	if err := tmp.Close(); err != nil {
		return Digest{}, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return Digest{}, err
	}
	return d, nil
}

// Open 打开已存的 tarball。
func (b *Blobs) Open(key string) (*os.File, error) {
	if !blobKey.MatchString(key) {
		return nil, fmt.Errorf("Invalid blob key %q", key)
	}
	return os.Open(b.path(key))
}

func (b *Blobs) Has(key string) bool {
	if !blobKey.MatchString(key) {
		return false
	}
	_, err := os.Stat(b.path(key))
	return err == nil
}

func (b *Blobs) path(key string) string {
	return filepath.Join(b.dir, key[:2], key[2:4], key)
}

// CheckIntegrity 校验 npm 元数据里声明的 integrity / shasum 与实际内容一致。
// integrity 可能带多个哈希（空格分隔），只认 sha512；没有 sha512 时退回 shasum。
func CheckIntegrity(d Digest, integrity, shasum string) error {
	if integrity != "" {
		for _, part := range strings.Fields(integrity) {
			if strings.HasPrefix(part, "sha512-") {
				if part == d.Integrity {
					return nil
				}
				return fmt.Errorf("sha512 mismatch: declared %s, actual %s", part, d.Integrity)
			}
		}
	}
	if shasum != "" {
		if shasum == d.Shasum {
			return nil
		}
		return fmt.Errorf("shasum mismatch: declared %s, actual %s", shasum, d.Shasum)
	}
	if integrity != "" {
		// 只有 sha1 / sha256 形式的 integrity、也没有 shasum：极老的包，无法用 sha512 校验
		return errors.New("integrity contains no sha512 and shasum is missing")
	}
	return errors.New("Metadata has neither integrity nor shasum, so it cannot be verified")
}
