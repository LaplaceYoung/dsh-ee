package netguard

import (
	"net"
	"testing"
)

func TestIsMetadataIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"169.254.169.254", true},
		{"169.254.170.2", true},
		{"100.100.100.200", true},
		{"::ffff:100.100.100.200", true},
		{"fd00:ec2::254", true},
		{"100.100.100.201", false},
		{"169.254.169.253", false},
		{"8.8.8.8", false},
		{"2606:4700:4700::1111", false},
	}
	for _, c := range cases {
		if got := IsMetadataIP(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("IsMetadataIP(%s) = %v，期望 %v", c.ip, got, c.want)
		}
	}
	if IsMetadataIP(nil) {
		t.Error("nil 不应命中元数据地址")
	}
}

func TestIsZeroNetwork(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"0.0.0.0", true},
		{"0.1.2.3", true},
		{"0.255.255.255", true},
		{"::", false}, // IPv6 未指定地址，由调用方的 IsUnspecified 拦
		{"1.0.0.0", false},
		{"10.0.0.5", false},
		{"100.100.100.200", false},
		{"127.0.0.1", false},
	}
	for _, c := range cases {
		if got := IsZeroNetwork(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("IsZeroNetwork(%s) = %v，期望 %v", c.ip, got, c.want)
		}
	}
	if IsZeroNetwork(nil) {
		t.Error("nil 不应命中 0.0.0.0/8")
	}
}

// MetadataIPs 返回副本：调用方改了也不能污染共享清单。
func TestMetadataIPsReturnsCopy(t *testing.T) {
	ips := MetadataIPs()
	if len(ips) == 0 {
		t.Fatal("元数据地址清单不能为空")
	}
	ips[0] = net.ParseIP("8.8.8.8")
	if !IsMetadataIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("修改返回值污染了共享清单")
	}
}
