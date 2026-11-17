package api

import "testing"

// KB 转换并发闸：名额占满时立即失败（接口据此回 429、不排队），释放一个后立刻能再占。
// 直接测信号量而不是靠并发请求撞：修好后一次转换是毫秒级，靠真实请求很难稳定地同时占满名额。
func TestKBConvertSlotLimit(t *testing.T) {
	var held []func()
	// 失败也要把占到的名额还回去，免得影响同包其它测试。
	t.Cleanup(func() {
		for _, release := range held {
			release()
		}
	})
	for i := 0; i < maxConcurrentKBConverts; i++ {
		release, ok := beginKBConvert()
		if !ok {
			t.Fatalf("第 %d 个名额应该能占到（上限 %d）", i+1, maxConcurrentKBConverts)
		}
		held = append(held, release)
	}
	if release, ok := beginKBConvert(); ok {
		release()
		t.Fatal("名额已满时不应该再占到")
	}
	// 转换失败/提前返回时 handler 用 defer release，名额必须能还回来、马上可以再占。
	first := held[0]
	held = held[1:]
	first()
	release, ok := beginKBConvert()
	if !ok {
		t.Fatal("释放名额后应该能再占到")
	}
	release()
}
