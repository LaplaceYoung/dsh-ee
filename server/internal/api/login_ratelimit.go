package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// 登录失败按客户端 IP 的固定窗口限流。
//
// 只统计「用户名或密码错误」；用户名锁定（15 分钟内连续失败 5 次锁 15 分钟）仍按用户名
// 单独生效，两者互不替代：用户名锁挡针对单个账号的爆破，IP 限流挡同一个出口对大量
// 账号的密码喷洒。公司内网常有很多人共用一个出口 IP，所以默认值取得比较宽
// （15 分钟 50 次），并允许用 DSH_EE_LOGIN_IP_LIMIT=0 关掉，避免把整个出口封死。
const (
	defaultLoginIPLimit  = 50
	defaultLoginIPWindow = 15 * time.Minute
	// 内存上限：来源 IP 可能很多，超过后先清过期窗口，仍然满就淘汰旧条目。
	maxLoginIPEntries = 10000
)

type loginIPLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	items  map[string]*rateWindow
}

// newLoginIPLimiter 建按 IP 的登录失败限流器。limit 为 0 时用默认值，小于 0 表示关闭
// （配置里 DSH_EE_LOGIN_IP_LIMIT=0/off 会传 -1）；window 为 0 时用默认值。
func newLoginIPLimiter(limit int, window time.Duration) *loginIPLimiter {
	if limit == 0 {
		limit = defaultLoginIPLimit
	}
	if window <= 0 {
		window = defaultLoginIPWindow
	}
	return &loginIPLimiter{limit: limit, window: window, now: time.Now, items: map[string]*rateWindow{}}
}

func (l *loginIPLimiter) enabled() bool { return l != nil && l.limit > 0 }

// blocked 判断这个 IP 在当前窗口里是否已经用完了失败次数，并返回距窗口重置还有多久。
func (l *loginIPLimiter) blocked(ip string) (time.Duration, bool) {
	if !l.enabled() {
		return 0, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.items[ip]
	if w == nil {
		return 0, false
	}
	elapsed := now.Sub(w.start)
	if elapsed >= l.window {
		delete(l.items, ip)
		return 0, false
	}
	if w.count < l.limit {
		return 0, false
	}
	return l.window - elapsed, true
}

// fail 记录一次密码错误。窗口内失败次数达到 limit 后，后续请求在 blocked 处被拒绝。
func (l *loginIPLimiter) fail(ip string) {
	if !l.enabled() || ip == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if w := l.items[ip]; w != nil {
		if now.Sub(w.start) < l.window {
			w.count++
			return
		}
		delete(l.items, ip)
	}
	if len(l.items) >= maxLoginIPEntries {
		l.pruneLocked(now)
		for len(l.items) >= maxLoginIPEntries {
			for k := range l.items {
				delete(l.items, k)
				break
			}
		}
	}
	l.items[ip] = &rateWindow{start: now, count: 1}
}

// pruneLocked 清掉已经过窗口的条目，调用方持有 l.mu。
func (l *loginIPLimiter) pruneLocked(now time.Time) {
	for k, w := range l.items {
		if now.Sub(w.start) >= l.window {
			delete(l.items, k)
		}
	}
}

// writeLoginRateLimited 写 429 并带上 Retry-After（秒，向上取整），客户端据此等待。
// code 是前端用的机器码，params 里同时给出 retry_after（秒），与 Retry-After 头一致。
func writeLoginRateLimited(w http.ResponseWriter, retry time.Duration, code string, params map[string]any, msg string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retry)))
	writeErrCode(w, http.StatusTooManyRequests, code, params, msg)
}

// retryAfterSeconds 把剩余等待时间换算成 Retry-After 的秒数，至少 1 秒。
func retryAfterSeconds(d time.Duration) int {
	s := int((d + time.Second - 1) / time.Second)
	if s < 1 {
		s = 1
	}
	return s
}
