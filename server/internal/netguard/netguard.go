// Package netguard 集中「服务端主动访问目标」里所有入口都不该放行的受限地址：
// 0.0.0.0/8（RFC 1122 "this network"）与已知云主机元数据地址。
//
// 上游 npm 源（registry）、企业目录 tarball 下载（directory）和连接试连（api）对
// 内网地址的放行语义不同——registry 有 DSH_EE_UPSTREAM_ALLOW_PRIVATE、目录有
// DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单、连接试连本来就允许内网 MCP——但这些地址
// 在任何一处都不该访问。清单写在一处，避免三份拷贝漂移：审计发现 registry 与
// directory 漏了 0.0.0.0/8 和阿里云元数据 100.100.100.200，而连接试连已经拦了。
package netguard

import "net"

// metadataIPs 是不在链路本地段里的已知云主机元数据地址；169.254.0.0/16 这类
// 段内地址由调用方现有的链路本地判定拦。
var metadataIPs = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS / GCP / Azure / Oracle 等
	net.ParseIP("169.254.170.2"),   // AWS ECS 任务凭据
	net.ParseIP("100.100.100.200"), // 阿里云 ECS
	net.ParseIP("fd00:ec2::254"),   // AWS 的 IPv6 元数据地址
}

// MetadataIPs 返回已知云主机元数据地址的副本，供调用方在自己的清单里复用。
func MetadataIPs() []net.IP {
	out := make([]net.IP, len(metadataIPs))
	copy(out, metadataIPs)
	return out
}

// IsMetadataIP 判断 ip 是不是已知的云主机元数据地址；IPv4-mapped 写法
// （::ffff:100.100.100.200）按同一地址处理。
func IsMetadataIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	for _, m := range metadataIPs {
		if v4 != nil {
			if mv4 := m.To4(); mv4 != nil && v4.Equal(mv4) {
				return true
			}
			continue
		}
		if ip.Equal(m) {
			return true
		}
	}
	return false
}

// IsZeroNetwork 判断 ip 是否落在 0.0.0.0/8（RFC 1122 "this network"）。
// net.IP.IsUnspecified 只覆盖 0.0.0.0 本身；0/8 的其它地址在部分系统上会被
// 路由到本机或直接不可达，不能当成公网目标。
func IsZeroNetwork(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 0
}
