package api

import "testing"

// TestMCPStreamLimiterAcquireRelease 覆盖计数器本身：上限判断、按用户隔离、归零删条目。
func TestMCPStreamLimiterAcquireRelease(t *testing.T) {
	var l mcpStreamLimiter
	if !l.acquire(7, 2) || !l.acquire(7, 2) {
		t.Fatal("上限内的两次占用都应成功")
	}
	if l.acquire(7, 2) {
		t.Fatal("第三次占用应超过上限")
	}
	if got := l.users[7]; got != 2 {
		t.Fatalf("被拒的占用不该改计数，实际 %d", got)
	}
	if !l.acquire(8, 2) {
		t.Fatal("别的用户不受影响")
	}
	l.release(7)
	if !l.acquire(7, 2) {
		t.Fatal("释放一个名额后应能再占用")
	}
	l.release(7)
	l.release(7)
	if _, ok := l.users[7]; ok {
		t.Fatal("计数归零后要删掉 map 条目")
	}
	if len(l.users) != 1 {
		t.Fatalf("只应剩用户 8，实际 %v", l.users)
	}
	l.release(999) // 多余的释放不能 panic，也不能凭空造条目
	if len(l.users) != 1 {
		t.Fatalf("多余的释放不该改表，实际 %v", l.users)
	}
}
