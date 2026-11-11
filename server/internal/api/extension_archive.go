package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/extension"
)

const (
	maxExtensionZIPBytes   = 8 << 20
	maxExtensionFileBytes  = 4 << 20
	maxExtensionFileCounts = 16
)

type extensionArchive struct {
	manifestRaw []byte
	openapiRaw  []byte
	openapiName string
	files       extension.Files
}

// readExtensionArchive 解一个声明式扩展 ZIP，同时限制路径、文件数和解压后体积。
// 只把清单与接口文档交给检查器；README、图标和应用文件在本次安装包中忽略，不执行任何内容。
func readExtensionArchive(body []byte) (*extensionArchive, error) {
	if len(body) == 0 || len(body) > maxExtensionZIPBytes {
		return nil, fmt.Errorf("扩展 ZIP 不能为空，且不能超过 %d MB", maxExtensionZIPBytes>>20)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, errors.New("不是合法的 ZIP 文件，请上传声明式扩展 zip 包")
	}
	if len(zr.File) > maxExtensionFileCounts {
		return nil, fmt.Errorf("扩展 ZIP 最多包含 %d 个文件", maxExtensionFileCounts)
	}
	out := &extensionArchive{files: extension.Files{}}
	var rawEntries = map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			continue
		}
		var head struct {
			OpenAPI string `json:"openapi"`
		}
		_ = head
		if !fs.ValidPath(name) || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("扩展 ZIP 里的路径不安全：%s", name)
		}
		if f.UncompressedSize64 > maxExtensionFileBytes {
			return nil, fmt.Errorf("扩展文件 %s 超过 %d MB", name, maxExtensionFileBytes>>20)
		}
		total += int64(f.UncompressedSize64)
		if total > maxExtensionZIPBytes {
			return nil, fmt.Errorf("扩展 ZIP 解压后不能超过 %d MB", maxExtensionZIPBytes>>20)
		}
		if _, exists := rawEntries[name]; exists {
			return nil, fmt.Errorf("扩展 ZIP 里重复了文件：%s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxExtensionFileBytes+1))
		closeErr := rc.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxExtensionFileBytes {
			return nil, fmt.Errorf("扩展文件 %s 超过 %d MB", name, maxExtensionFileBytes>>20)
		}
		rawEntries[name] = data
	}
	manifest, ok := rawEntries[extension.ManifestFile]
	if ok {
		out.manifestRaw = manifest
		out.files[extension.ManifestFile] = manifest
		var m struct {
			OpenAPI string `json:"openapi"`
		}
		if json.Unmarshal(manifest, &m) == nil && m.OpenAPI != "" {
			out.openapiName = m.OpenAPI
			if !fs.ValidPath(m.OpenAPI) || path.Clean(m.OpenAPI) != m.OpenAPI || strings.HasPrefix(m.OpenAPI, "/") || strings.Contains(m.OpenAPI, "\\") {
				return nil, errors.New("清单里的 OpenAPI 文件路径不安全")
			}
			if doc, exists := rawEntries[m.OpenAPI]; exists {
				out.openapiRaw = doc
				out.files[m.OpenAPI] = doc
			}
		}
	}
	if len(out.files) == 0 {
		// 即使缺清单也交给同一 Check，让它按标准 code 回报，而不是发散成另一套错误格式。
		out.files = extension.Files{}
	}
	return out, nil
}

func (a *extensionArchive) check() *extension.Result {
	return extension.Check(a.files)
}

func (a *extensionArchive) writePreview(w http.ResponseWriter, r *http.Request, res *extension.Result) {
	fields := []extensionInstallField{}
	if res.Manifest != nil {
		fields = extensionInstallFields(res.Manifest)
	}
	writeJSON(w, http.StatusOK, struct {
		*extension.Result
		InstallFields []extensionInstallField `json:"install_fields"`
	}{Result: res, InstallFields: fields})
}

// POST /api/admin/extensions/check：上传前预检，不联网、不运行扩展内容。
func (s *Server) handleExtensionCheck(w http.ResponseWriter, r *http.Request) {
	body, err := readRequestBytes(w, r, maxExtensionZIPBytes)
	if err != nil {
		return
	}
	arc, err := readExtensionArchive(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	arc.writePreview(w, r, arc.check())
}

func readRequestBytes(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, max+1)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("扩展 ZIP 不能超过 %d MB", max>>20))
			return nil, err
		}
		writeErr(w, http.StatusBadRequest, "读取上传内容失败："+err.Error())
		return nil, err
	}
	if int64(len(body)) > max {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("扩展 ZIP 不能超过 %d MB", max>>20))
		return nil, http.ErrBodyReadAfterClose
	}
	return body, nil
}
