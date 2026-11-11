package extension

import (
	"bytes"
	"io/fs"
	"path"
	"time"
)

// Files 是一个只读的内存文件系统。安装 ZIP 和测试共用同一份 Check，避免预检与运行使用不同解析路径。
type Files map[string][]byte

func (f Files) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	b, ok := f[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return b, nil
}

func (f Files) Open(name string) (fs.File, error) {
	b, err := f.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return &memoryFile{Reader: bytes.NewReader(b), info: memoryFileInfo{name: path.Base(name), size: int64(len(b))}}, nil
}

type memoryFile struct {
	*bytes.Reader
	info memoryFileInfo
}

func (f *memoryFile) Close() error               { return nil }
func (f *memoryFile) Stat() (fs.FileInfo, error) { return f.info, nil }

// memoryFileInfo 只为 ReadFile/Open 提供 Stat；扩展检查不需要目录遍历。
type memoryFileInfo struct {
	name string
	size int64
}

func (i memoryFileInfo) Name() string       { return i.name }
func (i memoryFileInfo) Size() int64        { return i.size }
func (i memoryFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (i memoryFileInfo) IsDir() bool        { return false }
func (i memoryFileInfo) Sys() any           { return nil }
