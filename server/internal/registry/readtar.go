package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// 审核时读包内容用：列文件、读单个文本文件、对文本文件跑一遍调用方给的扫描。
const (
	maxListedFiles = 300
	maxReviewScan  = 512 << 10
)

type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type FileContent struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// 没给出内容的原因：二进制、含疑似密钥、不是文本
	Withheld string `json:"withheld,omitempty"`
}

type PackageContents struct {
	PackageJSON []byte       `json:"-"`
	Total       int          `json:"total_files"`
	Files       []FileEntry  `json:"files"`
	Truncated   bool         `json:"files_truncated,omitempty"`
	File        *FileContent `json:"file,omitempty"`
}

// ReadTarball 解开 .tgz：返回文件清单（最多 300 个）与 package.json；want 非空时读出该文件（最多 maxBytes 字节）；
// scan 非空时对每个小于 512 KB 的文本文件调用一次（路径是包内相对路径）。
// 含疑似密钥的文件、.env 之类的文件不会给出内容。
func ReadTarball(r io.Reader, want string, maxBytes int, scan func(file string, b []byte)) (*PackageContents, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("Not gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := &PackageContents{Files: []FileEntry{}}
	root := ""
	var unpacked int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Unpacking failed: %w", err)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			continue
		}
		name := strings.TrimPrefix(path.Clean("/"+h.Name), "/")
		top, rel, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		if root == "" {
			root = top
		}
		if top != root {
			continue
		}
		out.Total++
		unpacked += h.Size
		if out.Total > maxFiles || unpacked > maxUnpackedSize {
			return nil, errors.New("Too many files or files too large after unpacking")
		}
		if len(out.Files) < maxListedFiles {
			out.Files = append(out.Files, FileEntry{Path: rel, Size: h.Size})
		} else {
			out.Truncated = true
		}
		base := path.Base(rel)
		isWanted := want != "" && rel == want
		switch {
		case rel == "package.json":
			if h.Size > maxPackageJSON {
				return nil, errors.New("package.json is too large")
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxPackageJSON))
			if err != nil {
				return nil, err
			}
			out.PackageJSON = b
			if scan != nil && !sensitive(base, b) {
				scan(rel, b)
			}
			if isWanted {
				out.File = fileContent(rel, h.Size, b, maxBytes)
			}
		case isWanted || (scan != nil && h.Size <= maxReviewScan && scannable(base)):
			limit := int64(maxReviewScan)
			if isWanted {
				limit = max(limit, int64(maxBytes))
			}
			b, err := io.ReadAll(io.LimitReader(tr, limit))
			if err != nil {
				return nil, err
			}
			if scan != nil && h.Size <= maxReviewScan && scannable(base) && !sensitive(base, b) {
				scan(rel, b)
			}
			if isWanted {
				out.File = fileContent(rel, h.Size, b, maxBytes)
			}
		}
	}
	if out.PackageJSON == nil {
		return nil, ErrNoPackageJSON
	}
	if want != "" && out.File == nil {
		out.File = &FileContent{Path: want, Withheld: "包里没有这个文件，路径要写成清单里的相对路径"}
	}
	return out, nil
}

// sensitive 判断文件是不是密钥文件或里面有疑似密钥：这样的文件不交给调用方扫描，免得可疑点的「证据」里带出密钥原文。
func sensitive(base string, b []byte) bool {
	for _, f := range secretFiles {
		if f.match(base) {
			return true
		}
	}
	for _, p := range secretPatterns {
		if p.re.Match(b) {
			return true
		}
	}
	return false
}

func fileContent(rel string, size int64, b []byte, maxBytes int) *FileContent {
	fc := &FileContent{Path: rel, Size: size}
	base := path.Base(rel)
	for _, f := range secretFiles {
		if f.match(base) {
			fc.Withheld = "这类文件通常含密钥（" + f.kind + "），不展示内容；审核时只需确认它不该出现在包里"
			return fc
		}
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b[:min(len(b), 4096)]) {
		fc.Withheld = "二进制文件，不展示内容"
		return fc
	}
	for _, p := range secretPatterns {
		if p.re.Match(b) {
			fc.Withheld = "文件里有疑似密钥（" + p.kind + "），不展示内容"
			return fc
		}
	}
	if len(b) > maxBytes {
		b, fc.Truncated = b[:maxBytes], true
		for !utf8.Valid(b) && len(b) > 0 {
			b = b[:len(b)-1]
		}
	}
	fc.Text = string(b)
	return fc
}
