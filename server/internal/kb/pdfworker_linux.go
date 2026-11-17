//go:build linux

package kb

import "syscall"

// pdfWorkerDataLimit 是子进程的硬内存上限（数据段 + 匿名可写映射，即 Go 真正用掉的内存，
// 不含运行时预留的虚拟地址空间）。超过之后 Go runtime 直接 fatal，只炸这个子进程。
// 128 MB 的 GOMEMLIMIT + 384 MB 硬上限：正常 PDF 的转换远低于它，恶意 PDF 到不了服务进程。
const pdfWorkerDataLimit = 384 << 20

// setPDFWorkerMemoryLimit 设软上限（GOMEMLIMIT，所有平台）+ Linux 上的 RLIMIT_DATA 硬上限。
func setPDFWorkerMemoryLimit() {
	setPDFWorkerMemoryLimitCommon()
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_DATA, &lim); err != nil {
		return
	}
	// 容器/父进程已经给了更紧的上限时尊重它，不要反过来放宽。
	if lim.Max != ^uint64(0) && lim.Max < pdfWorkerDataLimit {
		return
	}
	lim.Cur, lim.Max = pdfWorkerDataLimit, pdfWorkerDataLimit
	_ = syscall.Setrlimit(syscall.RLIMIT_DATA, &lim)
}
