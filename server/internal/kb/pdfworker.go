package kb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// PDF 文字抽取的子进程隔离。
//
// 为什么需要它：ledongthuc/pdf 解压内容流、解析文字时没有任何内存上限，单页解析过程中也插不进
// ctx 检查。预扫描（pdfscan.go）能拦掉「一条流解压成几 GB」这种最直接的放大，但构造得巧的 PDF
// 仍能把进程撑到几百 MB（一页几十万个文字片段、正文刚好卡在上限内的一次大分配）。这些分配如果
// 发生在服务进程里，量大了就是 OOM——runtime 的 fatal error，recover 兜不住，整个服务没了。
//
// 所以 PDF 的解析放到本进程的可执行文件重新拉起的子进程里：子进程设 GOMEMLIMIT +（Linux）
// RLIMIT_DATA 硬上限，超了它自己 fatal 退出，父进程只收到「转换失败」，服务进程安然无恙。
// 父进程仍然先做检查再拿结果：子进程退出码非零、输出超长都按「内容太多/文件太大」拒收。
//
// 兼容性：只有 cmd/dsh-ee 的 main 调用 EnablePDFIsolation。测试或其它嵌入方不调用时走进程内
// 路径（pdfscan.go 的预扫描 + 正文预算仍然生效，只是少了硬内存上限这一层）；
// 子进程拉不起来时也退回进程内，避免因为环境问题整块功能不可用。

const (
	// PDFWorkerSubcommand 是子进程的隐藏子命令，由 cmd/dsh-ee 的 runCLI 分发，不要写进用户文档。
	PDFWorkerSubcommand = "__kb-pdf-worker"
	// 子进程最多收多大的 PDF（接口本身限 50 MB，这里是防御性上限）。
	maxPDFWorkerInput = 64 << 20
	// 子进程 JSON 结果的上限：正文预算 16 MB，JSON 转义（引号/反斜杠）最多再翻一倍，留点余量。
	maxPDFWorkerResponse = 40 << 20
	// 子进程的软内存上限（GOMEMLIMIT）：正常转换碰不到，恶意文件会让 GC 提前介入。
	pdfWorkerGOMEMLimit = 128 << 20
	// 子进程自己的超时：父进程的 ctx 到点会 kill 它；万一父进程先没了，它也不能一直跑。
	pdfWorkerTimeout = 120 * time.Second
	// 子进程的环境变量标记：防止「隔离打开 → 子进程二进制不认隐藏子命令 → 又调 Convert」时无限套娃。
	pdfWorkerChildEnv = "DSH_EE_KB_PDF_WORKER_CHILD"
)

// pdfIsolation 由 main 打开：打开后 PDF 解析在子进程里做。
var pdfIsolation atomic.Bool

// EnablePDFIsolation 打开 PDF 子进程隔离。由 cmd/dsh-ee 的 main 在启动服务前调用。
func EnablePDFIsolation() { pdfIsolation.Store(true) }

// pdfWorkerCommand 造子进程命令；测试里会替换成「测试二进制 + 环境变量」的辅助进程。
var pdfWorkerCommand = func(ctx context.Context) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, exe, PDFWorkerSubcommand), nil
}

// pdfWorkerResponse 是子进程的输出：要么 Markdown + Info，要么一句错误。
type pdfWorkerResponse struct {
	Markdown string `json:"markdown,omitempty"`
	Info     *Info  `json:"info,omitempty"`
	Error    string `json:"error,omitempty"`
}

// convertPDFEntry 是 .pdf 的入口：隔离打开时走子进程，否则（测试、嵌入方、子进程拉不起来）进程内解析。
func convertPDFEntry(ctx context.Context, data []byte) (*Converted, error) {
	// 已经在 PDF 子进程里就不要再套一层（嵌入方没有分发隐藏子命令时的保险）。
	if !pdfIsolation.Load() || os.Getenv(pdfWorkerChildEnv) == "1" {
		return convertPDF(ctx, data)
	}
	out, err := convertPDFIsolated(ctx, data)
	if err != nil && errors.Is(err, errPDFWorkerUnavailable) {
		return convertPDF(ctx, data)
	}
	return out, err
}

// errPDFWorkerUnavailable 表示子进程没起来（环境问题），调用方退回进程内解析。
var errPDFWorkerUnavailable = errors.New("PDF 转换子进程不可用")

// convertPDFIsolated 在子进程里解析 PDF：输入走 stdin，结果（JSON）走 stdout。
func convertPDFIsolated(ctx context.Context, data []byte) (*Converted, error) {
	cmd, err := pdfWorkerCommand(ctx)
	if err != nil {
		return nil, errPDFWorkerUnavailable
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errPDFWorkerUnavailable
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errPDFWorkerUnavailable
	}
	// 子进程里再看到 Convert 时不再套娃（见 convertPDFEntry）。
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, pdfWorkerChildEnv+"=1")
	cmd.Stderr = io.Discard
	// 被 kill 之后最多再等 2 秒收尾（管道可能还没关），避免一次转换卡住 HTTP 请求。
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, errPDFWorkerUnavailable
	}
	go func() {
		_, _ = stdin.Write(data)
		_ = stdin.Close()
	}()
	// 先读 stdout 再 Wait，避免子进程写满管道时双方卡死。
	resp, readErr := io.ReadAll(io.LimitReader(stdout, maxPDFWorkerResponse+1))
	if len(resp) > maxPDFWorkerResponse {
		// 结果超长：先把子进程收掉，否则它写不进管道会一直卡到超时。
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, checkCtx(ctx)
	}
	if readErr != nil || waitErr != nil {
		// 子进程被内存上限杀掉（exit status 2 / signal）、或没写出完整结果：
		// 对用户来说就是「这份 PDF 太大/太复杂」，不能把内部细节抛出去。
		return nil, errPDFTooBig
	}
	if len(resp) > maxPDFWorkerResponse {
		return nil, errTooBig
	}
	var r pdfWorkerResponse
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, errors.New("PDF 解析失败：转换进程返回了无法识别的内容")
	}
	if r.Error != "" {
		return nil, errors.New(r.Error)
	}
	if r.Info == nil {
		return nil, errors.New("PDF 解析失败：转换进程没有返回结果")
	}
	if len(r.Markdown) > maxOutput {
		return nil, errTooBig
	}
	return &Converted{Markdown: r.Markdown, Mime: "application/pdf", Info: *r.Info}, nil
}

// PDFWorkerMain 是子进程入口：stdin 是 PDF 原文，stdout 是 JSON 结果。
// 由 cmd/dsh-ee 的隐藏子命令调用；运行期间设好内存上限，任何 OOM 都只炸这个子进程。
func PDFWorkerMain(in io.Reader, out io.Writer) int {
	setPDFWorkerMemoryLimit()
	data, err := io.ReadAll(io.LimitReader(in, maxPDFWorkerInput+1))
	var resp pdfWorkerResponse
	switch {
	case err != nil:
		resp.Error = errPDFTooBig.Error()
	case len(data) > maxPDFWorkerInput:
		resp.Error = errPDFTooBig.Error()
	default:
		ctx, cancel := context.WithTimeout(context.Background(), pdfWorkerTimeout)
		conv, cerr := convertPDF(ctx, data)
		cancel()
		if cerr != nil {
			resp.Error = cerr.Error()
		} else {
			resp.Markdown = conv.Markdown
			resp.Info = &conv.Info
		}
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
	return 0
}

// setPDFWorkerMemoryLimitCommon 是所有平台都生效的软上限（GOMEMLIMIT）；Linux 上还有 RLIMIT_DATA 硬上限，
// 见 pdfworker_linux.go / pdfworker_other.go。
func setPDFWorkerMemoryLimitCommon() {
	debug.SetMemoryLimit(pdfWorkerGOMEMLimit)
}
