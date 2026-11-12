package kb

import (
	"context"
	"testing"
)

// 固定的小型检索评测集：不是完整基准，而是给搜索排序设一条可回归的底线。
// 语料是合成制度文档；每条问题都标明期望文档，评 recall@3 与 MRR。
var evalDocs = []*Doc{
	{ID: 101, Base: 1, Title: "员工手册", Folder: "人事", Version: 1, Text: `# 员工手册

## 年假

入职满一年有 5 天年假，满三年有 10 天年假。

## 病假

病假需要医院证明，并在当天向主管报备。

## 考勤

每天 9:30 前打卡，迟到超过 30 分钟按事假处理。
`},
	{ID: 102, Base: 1, Title: "差旅报销制度", Folder: "财务/差旅", Version: 1, Text: `# 差旅报销制度

## 住宿

一线城市住宿标准每晚不超过 600 元，其他城市不超过 400 元。

## 交通

高铁二等座；单程超过 6 小时可以坐飞机经济舱。

## 发票

住宿发票要写公司全称和入住日期，否则财务退回。
`},
	{ID: 103, Base: 1, Title: "IT 安全规范", Folder: "IT", Version: 1, Text: `# IT 安全规范

## VPN

员工离职当天回收 VPN 权限。

## 密码

密码至少 12 位，每 90 天更换一次。

## 设备

公司电脑不得安装来源不明的软件。
`},
}

var evalProduct = []*Doc{
	{ID: 201, Base: 2, Title: "产品 API 文档", Folder: "产品", Version: 1, Text: `# 产品 API 文档

## 限流

每个租户默认 100 QPS，超过后返回 429。

## 鉴权

请求头使用 Bearer token 鉴权。

## 错误码

429 表示触发限流，请退避后重试。
`},
}

func TestRetrievalEval(t *testing.T) {
	ix := NewIndex()
	ix.Load(1, "eval", evalDocs)
	ix.Load(2, "eval", evalProduct)
	ctx := context.Background()

	cases := []struct {
		query string
		want  int64
	}{
		{"年假 几天", 101},
		{"病假 医院证明", 101},
		{"住宿 标准 一线城市", 102},
		{"高铁 飞机 6 小时", 102},
		{"住宿发票 抬头", 102},
		{"VPN 离职 权限", 103},
		{"密码 长度 更换", 103},
		{"API 限流 QPS", 201},
		{"429 错误", 201},
		{"Bearer token 鉴权", 201},
		{"出差 住宿 多少钱", 102},
		{"迟到 怎么算", 101},
		{"公司电脑 安装 软件", 103},
		{"租户 QPS 限制", 201},
		{"限流 退避 重试", 201},
	}
	top1, rr, n := 0, 0.0, 0
	for _, tc := range cases {
		res, err := ix.Search(ctx, Query{Text: tc.query, Bases: []int64{1, 2}, Limit: 10})
		if err != nil {
			t.Fatalf("%q：%v", tc.query, err)
		}
		rank := 0
		for i, hit := range res.Hits {
			if hit.Doc == tc.want {
				rank = i + 1
				break
			}
		}
		if rank == 0 || rank > 3 {
			t.Errorf("%q：期望文档 %d 未进前 3（命中：%v）", tc.query, tc.want, hitDocs(res.Hits))
			continue
		}
		n++
		if rank == 1 {
			top1++
		}
		rr += 1 / float64(rank)
	}
	// 评测集的底线：所有问题至少进前 3；top-1 ≥ 70%，MRR ≥ 0.75。
	// 数值刻意不设满，给排序调优留空间；但不能跌破，否则说明检索明显退化。
	if n != len(cases) {
		t.Fatalf("有效评测 %d/%d", n, len(cases))
	}
	precision := float64(top1) / float64(len(cases))
	mrr := rr / float64(len(cases))
	if precision < 0.7 || mrr < 0.75 {
		t.Fatalf("检索退化：top-1=%.2f MRR=%.2f", precision, mrr)
	}
	t.Logf("top-1=%.2f MRR=%.2f（%d 条）", precision, mrr, len(cases))
}

func hitDocs(hits []Hit) []int64 {
	out := make([]int64, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Doc)
	}
	return out
}
