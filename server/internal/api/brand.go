package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

const maxBrandLogoSize = 256 << 10

type dshBrand struct {
	Enabled  bool   `json:"enabled"`
	Name     string `json:"name"`
	LogoURL  string `json:"logo_url"`
	Revision string `json:"revision"`
	// 以下字段旧版接入插件不认识，会忽略
	Title     string `json:"title,omitempty"`
	LogoShape string `json:"logo_shape,omitempty"`
	// 设计变量 → 浅色 / 深色取值，接入插件交给 DSH 的主题服务
	Theme map[string]store.BrandColor `json:"theme,omitempty"`
}

func (s *Server) brandInfo(ctx context.Context, base string) (dshBrand, error) {
	set, err := s.appSettings(ctx)
	if err != nil {
		return dshBrand{}, err
	}
	name := strings.TrimSpace(set.Brand.Name)
	if name == "" {
		name = strings.TrimSpace(set.OrgName)
	}
	if name == "" {
		name = "企业"
	}
	brand := dshBrand{Enabled: set.Brand.Enabled, Name: name, Revision: set.Brand.LogoRev}
	if brand.Enabled {
		brand.Title = set.Brand.Title
		brand.LogoShape = set.Brand.LogoShape
		if tokens := brandThemeTokens(set.Brand.Theme); len(tokens) > 0 {
			brand.Theme = tokens
		}
	}
	if brand.Enabled && brand.Revision != "" && base != "" {
		brand.LogoURL = base + "/api/dsh/brand/logo?rev=" + url.QueryEscape(brand.Revision)
	}
	return brand, nil
}

var (
	brandSVGEvent = regexp.MustCompile(`(?i)(?:^|[\s<])on[a-z0-9_-]+\s*=`)
	brandSVGStart = regexp.MustCompile(`(?i)<svg(?:\s|>)`)
)

func validateBrandLogo(data []byte) (string, error) {
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" {
			return "", errors.New("PNG 文件内容无效")
		}
		if cfg.Width != cfg.Height || cfg.Width < 64 {
			return "", errors.New("PNG 必须是边长至少 64 像素的正方形")
		}
		return "png", nil
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	lower := strings.ToLower(string(trimmed))
	if !brandSVGStart.Match(trimmed) {
		return "", errors.New("只接受 PNG 或 SVG 文件")
	}
	for _, bad := range []string{"<script", "<foreignobject", "javascript:", "<!doctype", "<!entity", "@import", "url("} {
		if strings.Contains(lower, bad) {
			return "", errors.New("SVG 含不安全内容，请上传 PNG")
		}
	}
	if brandSVGEvent.Match(trimmed) {
		return "", errors.New("SVG 含不安全内容，请上传 PNG")
	}
	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	root := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errors.New("SVG 文件内容无效，请上传 PNG")
		}
		switch v := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(v.Name.Local)
			if !root {
				root = name == "svg"
				if !root {
					return "", errors.New("SVG 根元素无效")
				}
			}
			if name == "script" || name == "foreignobject" {
				return "", errors.New("SVG 含不安全内容，请上传 PNG")
			}
			for _, attr := range v.Attr {
				name := strings.ToLower(attr.Name.Local)
				value := strings.TrimSpace(attr.Value)
				if strings.HasPrefix(name, "on") || strings.Contains(strings.ToLower(value), "javascript:") {
					return "", errors.New("SVG 含不安全内容，请上传 PNG")
				}
				if name == "href" && !strings.HasPrefix(value, "#") {
					return "", errors.New("SVG 含外部引用，请上传 PNG")
				}
			}
		case xml.Directive:
			return "", errors.New("SVG 含不安全声明，请上传 PNG")
		case xml.ProcInst:
			if strings.EqualFold(v.Target, "xml-stylesheet") {
				return "", errors.New("SVG 含外部样式，请上传 PNG")
			}
		}
	}
	if !root {
		return "", errors.New("SVG 根元素无效")
	}
	return "svg", nil
}

func (s *Server) handleUploadBrandLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBrandLogoSize+64<<10)
	if err := r.ParseMultipartForm(maxBrandLogoSize); err != nil {
		writeErr(w, http.StatusBadRequest, "请用 multipart 上传不超过 256 KB 的 logo")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少 file 上传字段")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBrandLogoSize+1))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if len(data) > maxBrandLogoSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "logo 不能超过 256 KB")
		return
	}
	ext, err := validateBrandLogo(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	dir := filepath.Join(s.opts.DataDir, "brand")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.internalErr(w, r, err)
		return
	}
	tmp, err := os.CreateTemp(dir, ".logo-*")
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	// 不要用 if err := …：那样会遮住外层 err，Chmod / Write 失败就被悄悄吞掉，留下一个空的或半截的 logo
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	final := filepath.Join(dir, "logo."+ext)
	if err := os.Rename(tmpName, final); err != nil {
		s.internalErr(w, r, err)
		return
	}
	other := "svg"
	if ext == "svg" {
		other = "png"
	}
	_ = os.Remove(filepath.Join(dir, "logo."+other))
	rev := hex.EncodeToString(func() []byte { sum := sha256.Sum256(data); return sum[:] }())[:12]
	set, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	set.Brand.LogoRev = rev
	if err := s.store.SaveSettings(r.Context(), set, currentUser(r).ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.settings.mu.Lock()
	s.settings.v = nil
	s.settings.mu.Unlock()
	s.kickConnect()
	s.audit(r, auditBrandUpdate, "logo", map[string]any{"revision": rev, "type": ext, "size": len(data)})
	writeJSON(w, http.StatusOK, map[string]string{"logo_rev": rev})
}

func (s *Server) handleBrandLogo(w http.ResponseWriter, r *http.Request) {
	for _, ext := range []string{"png", "svg"} {
		data, err := os.ReadFile(filepath.Join(s.opts.DataDir, "brand", "logo."+ext))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			s.internalErr(w, r, err)
			return
		}
		if ext == "png" {
			w.Header().Set("Content-Type", "image/png")
		} else {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	writeErr(w, http.StatusNotFound, "logo 不存在")
}
