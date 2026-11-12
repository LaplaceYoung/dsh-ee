package api

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/mcp"
	"github.com/hikariming/dsh-ee/server/internal/store"
)

// DSH 首屏启动台与「工作台」系统应用（第一版 M6）。
// 应用的工作台组件可以加 "home": true（每个应用最多 2 个，只限带数据源的只读组件），
// 系统应用的「我的待办」把全员勾选的首页组件汇总到一个窗格；home__ 工具提供我的应用、
// 新上线和本月用量数据；插件首屏用同一份目录铺开卡片。

const (
	// homeConn 是首页数据工具在网关上的「系统连接」名：工具名 home__<工具>。
	homeConn = "home"
	// workbenchHomeName 是预置「工作台」系统应用的标识（DSH 里的 /workbench）。
	workbenchHomeName = "workbench"
	// appMaxHomeWidgets 是单个应用能勾选的首页组件上限。
	appMaxHomeWidgets = 2
	// homeNewDays 是「新上线」的窗口天数。
	homeNewDays = 14
)

// wbHomeTypes 是可以挂到首页的组件类型（都要有只读数据源）。
var wbHomeTypes = map[string]bool{"metric": true, "table": true, "chart": true, "checklist": true}

// checkHomeWidgets 校验 home 标记：最多 2 个、只限带数据源的只读组件。
func checkHomeWidgets(wb *appWorkbench) string {
	n := 0
	for _, p := range wb.Panes {
		for _, w := range p.Widgets {
			if !w.Home {
				continue
			}
			n++
			if !wbHomeTypes[w.Type] || w.Source == nil {
				return fmt.Sprintf("组件「%s」不能显示在首页：只有带数据源的指标 / 表格 / 图表 / 清单可以", w.Title)
			}
		}
	}
	if n > appMaxHomeWidgets {
		return fmt.Sprintf("显示在首页的组件最多 %d 个，现在有 %d 个", appMaxHomeWidgets, n)
	}
	return ""
}

// homeWidgets 收集所有已启用应用里勾选上首页的组件，标题带上应用名，源已按当前员工解析。
func homeWidgets(c *dshCatalog) []wbWidget {
	var out []wbWidget
	for _, app := range c.Apps {
		if app.Workbench == nil {
			continue
		}
		for _, p := range app.Workbench.Panes {
			for _, w := range p.Widgets {
				if !w.Home || w.Source == nil {
					continue
				}
				w.Home = false
				w.Title = app.Title + " · " + w.Title
				out = append(out, w)
			}
		}
	}
	return out
}

// homeToolDefs 是 home__ 只读工具的清单。
func homeToolDefs() []map[string]any {
	def := func(name, desc string) map[string]any {
		return map[string]any{"name": homeConn + "__" + name, "description": "【工作台】" + desc,
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			"annotations": map[string]any{"readOnlyHint": true}}
	}
	return []map[string]any{
		def("apps", "我装了哪些应用：标识、标题、图标、我近 30 天用的次数。"),
		def("new", "最近 14 天新上线的应用。"),
		def("usage", "我这个月的用量与额度。"),
	}
}

// homeCall 执行 home__ 工具。
func (s *Server) homeCall(ctx context.Context, v viewer, c *dshCatalog, name string) json.RawMessage {
	switch name {
	case homeConn + "__apps":
		usage, err := s.store.UserAssetUsage(ctx, v.user.ID, time.Now().AddDate(0, 0, -30))
		if err != nil {
			s.log.Warn("取我的应用用量失败", "err", err)
			usage = map[int64]int64{}
		}
		items := make([]map[string]any, 0, len(c.Apps))
		for _, app := range c.Apps {
			items = append(items, map[string]any{"name": app.Name, "title": app.Title, "icon": app.Icon, "uses": usage[app.AssetID]})
		}
		return jsonText(map[string]any{"items": items}, false)
	case homeConn + "__new":
		cut := time.Now().AddDate(0, 0, -homeNewDays)
		items := []map[string]any{}
		for _, app := range c.Apps {
			if a := c.assets[app.Name]; a != nil && a.CreatedAt.After(cut) {
				items = append(items, map[string]any{"name": app.Name, "title": app.Title, "icon": app.Icon, "created_at": a.CreatedAt})
			}
		}
		return jsonText(map[string]any{"items": items}, false)
	case homeConn + "__usage":
		return s.myUsageJSON(ctx, v)
	}
	return mcp.TextResult("没有这个工具："+name, true)
}

// workbenchHomeApp 生成「工作台」系统应用：右侧面板是我的待办（首页组件汇总）、我的应用、
// 本月用量、新上线；左边对话是「总助理」，会把问题引到合适的应用。
func workbenchHomeApp(c *dshCatalog, base string) (*dshApp, *dshSkill, string) {
	todo := homeWidgets(c)
	panes := []wbPane{{
		ID: "todo", Type: "panel", Title: "我的待办",
		Widgets: append([]wbWidget{{Type: "markdown", Text: "你在各应用里待处理的数字和清单，来自应用作者勾选的首页组件。"}}, todo...),
	}, {
		ID: "apps", Type: "panel", Title: "我的应用",
		Widgets: []wbWidget{{Type: "table", Title: "我的应用", Rows: "items",
			Source:  &wbSource{Connection: homeConn, Tool: "apps"},
			Columns: []wbColumn{{Field: "icon", Title: ""}, {Field: "title", Title: "应用"}, {Field: "uses", Title: "近 30 天我用过"}}}},
	}, {
		ID: "usage", Type: "panel", Title: "本月用量",
		Widgets: []wbWidget{{Type: "metric", Title: "本月用量", Field: "used", Format: "number",
			Source: &wbSource{Connection: homeConn, Tool: "usage"}}},
	}, {
		ID: "new", Type: "panel", Title: "新上线",
		Widgets: []wbWidget{{Type: "table", Title: fmt.Sprintf("最近 %d 天新上线", homeNewDays), Rows: "items",
			Source:  &wbSource{Connection: homeConn, Tool: "new"},
			Columns: []wbColumn{{Field: "icon", Title: ""}, {Field: "title", Title: "应用"}, {Field: "created_at", Title: "上线时间", Format: "date"}},
			Empty:   &wbEmpty{Text: "最近没有新上线的应用"}}},
	}}
	wb := &appWorkbench{Panes: panes}

	var body strings.Builder
	body.WriteString("# 工作台\n\n你是员工的「总助理」。帮员工看清今天要处理什么、自己有哪些应用、企业最近上了什么新能力；员工的问题如果在某个应用的领域里，就引导他用那个应用（说「你可以用 /应用标识」），不要自己替别的应用回答专业问题。\n")
	body.WriteString("\n## 用户可以这样问\n\n- 我今天有什么要处理的？\n- 我有哪些应用？最近哪个用得多？\n- 最近新上了什么应用？\n")
	body.WriteString(workbenchContent(wb))
	body.WriteString(fmt.Sprintf("\n调用 `ee_workbench` 时 app 写 `%s`。\n", workbenchHomeName))
	content := body.String()
	version := int(crc32.ChecksumIEEE([]byte(content))%900000) + 1
	title, desc := "工作台", "我的待办、我的应用、本月用量和新上线，问题不知道找谁就问总助理"
	starters := []string{"我今天有什么要处理的？", "我最近用哪个应用最多？", "最近新上了什么应用？"}
	app := &dshApp{Name: workbenchHomeName, Title: title, Description: desc, Icon: "🏠", Starters: starters, Launch: "new",
		Delivery: store.DeliveryAuto, Version: version, Workbench: wb}
	skill := &dshSkill{Name: workbenchHomeName, Title: title, Kind: store.KindApp, Description: desc, UserInvocable: true, Version: version}
	_ = base
	return app, skill, content
}

// myUsageJSON 是 home__usage 的结果：本月用量与额度。
func (s *Server) myUsageJSON(ctx context.Context, v viewer) json.RawMessage {
	used, err := s.store.MonthTokensByUser(ctx, v.user.ID)
	if err != nil {
		return mcp.TextResult("取用量失败："+err.Error(), true)
	}
	out := map[string]any{"used": used}
	if v.user.MonthlyTokenLimit != nil {
		out["limit"] = *v.user.MonthlyTokenLimit
	}
	return jsonText(out, false)
}
