package registry

import "path"

// 给 tarball 之外的上传（技能包等）复用的检查：疑似密钥、不该带的文件、安全的相对路径。

// ScanText 返回一段文本里命中的疑似密钥类型；二进制内容直接跳过。
func ScanText(file string, b []byte) []SecretHit {
	var info TarballInfo
	info.scan(file, b)
	return info.Secrets
}

// SecretFileKind 判断文件名是不是不该出现在包里的文件（.env、私钥等），返回类型；不是返回空串。
func SecretFileKind(name string) string {
	base := path.Base(name)
	for _, f := range secretFiles {
		if f.match(base) {
			return f.kind
		}
	}
	return ""
}

// SafeRelPath 判断包内路径是否安全（相对路径、不含 .. 等）。
func SafeRelPath(p string) bool { return safeRelPath(p) }
