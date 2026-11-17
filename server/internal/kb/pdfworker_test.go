package kb

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// PDF 抽取子进程隔离的测试。
//
// 子进程就是「测试二进制自己 + 一个环境变量」：TestMain 看到 DSH_EE_KB_PDF_WORKER_TEST=worker 时
// 不跑测试，直接执行 PDFWorkerMain（和生产里 cmd/dsh-ee 的隐藏子命令走同一段代码）。
const (
	workerTestMode   = "DSH_EE_KB_PDF_WORKER_TEST"
	workerTestMarker = "DSH_EE_KB_PDF_WORKER_MARKER"
)

func TestMain(m *testing.M) {
	switch os.Getenv(workerTestMode) {
	case "worker":
		code := PDFWorkerMain(os.Stdin, os.Stdout)
		// 给父进程留个记号，证明结果确实是子进程算出来的。
		if p := os.Getenv(workerTestMarker); p != "" {
			_ = os.WriteFile(p, []byte("1"), 0o600)
		}
		os.Exit(code)
	case "crash":
		// 模拟子进程被内存上限杀掉 / 解析崩溃：没有输出、退出码非零。
		os.Exit(2)
	}
	os.Exit(m.Run())
}

// usePDFIsolation 把子进程命令换成测试二进制的辅助进程，并临时打开隔离。
func usePDFIsolation(t *testing.T, mode, marker string) {
	t.Helper()
	prev := pdfWorkerCommand
	pdfWorkerCommand = func(ctx context.Context) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, os.Args[0])
		env := append(os.Environ(), workerTestMode+"="+mode)
		if marker != "" {
			env = append(env, workerTestMarker+"="+marker)
		}
		cmd.Env = env
		return cmd, nil
	}
	EnablePDFIsolation()
	t.Cleanup(func() {
		pdfWorkerCommand = prev
		pdfIsolation.Store(false)
	})
}

// 打开隔离后，正常 PDF 仍然能转出来，而且结果确实是子进程算的（有 marker 文件为证）。
func TestPDFIsolationSubprocess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "worker-ran")
	usePDFIsolation(t, "worker", marker)
	data := makePDFPages(pdfSpec{lines: []string{"Travel policy: hotel limit is 600 per night.", "Flights need manager approval."}})
	out, err := Convert(context.Background(), "policy.pdf", data, Options{})
	if err != nil {
		t.Fatalf("隔离子进程里正常 PDF 应该能转：%v", err)
	}
	if !strings.Contains(out.Markdown, "hotel limit is 600") || out.Info.Format != "pdf" {
		t.Fatalf("子进程返回的正文不对：%+v %q", out.Info, out.Markdown)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("结果不是子进程算出来的：%v", err)
	}
}

// 子进程崩溃/被内存上限杀掉时，父进程要立刻转成「内容太多或文件太大」，不能把服务进程带走。
func TestPDFIsolationWorkerCrashContained(t *testing.T) {
	usePDFIsolation(t, "crash", "")
	data := makePDFPages(pdfSpec{lines: []string{"hello world"}})
	start := time.Now()
	out, err := Convert(context.Background(), "crash.pdf", data, Options{})
	if out != nil {
		t.Fatalf("子进程崩溃时不应该有结果")
	}
	if err == nil || !strings.Contains(err.Error(), "内容太多") || !strings.Contains(err.Error(), "文件太大") {
		t.Fatalf("子进程崩溃应转成「内容太多或文件太大」：%v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("子进程崩溃后应立刻返回，实际 %v", elapsed)
	}
}

// 压缩放大 PDF 走子进程时也在预扫描阶段就被拒收（错误信息由子进程传回，父进程原样返回）。
func TestPDFIsolationBombRejected(t *testing.T) {
	usePDFIsolation(t, "worker", "")
	data, filters := flateLiteral(32<<20, false)
	pdf := pdfOnePage(t, []pdfContent{{data: data, filters: filters}})
	start := time.Now()
	out, err := Convert(context.Background(), "bomb.pdf", pdf, Options{})
	if out != nil || err == nil || !strings.Contains(err.Error(), "内容太多") {
		t.Fatalf("子进程里也应该拒收压缩放大 PDF：out=%v err=%v", out, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("拒收太慢：%v", elapsed)
	}
}
