//go:build !linux

package kb

// setPDFWorkerMemoryLimit 在非 Linux 平台只有 GOMEMLIMIT 软上限（没有可用的 RLIMIT_DATA 硬上限），
// 子进程仍然能靠父进程的超时 kill 收住，只是极端构造的 PDF 可能先占不少内存。
func setPDFWorkerMemoryLimit() {
	setPDFWorkerMemoryLimitCommon()
}
