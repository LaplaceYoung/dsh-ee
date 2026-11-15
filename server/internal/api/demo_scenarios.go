package api

import (
	"context"
	"errors"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例场景：除了示例公司「星河科技」，还有小团队的场景（先做了一个写小说的工作室）。
// 管理员导入时选一个；可以都导入，互相看不到对方的应用（见 demoIsolate）。设定见 docs/demo-company.md。

const (
	demoScenarioCompany = "xinghe"
	demoScenarioNovel   = "novel"
)

type demoScenario struct {
	ID string `json:"id"`
	// company：有部门、有业务系统的公司；team：几个人的小团队
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Summary string `json:"summary"`

	users    []demoUser
	kbases   []demoKBase
	assets   func() []demoAsset
	selfApps func() []demoSelfApp
	guide    func() []demoGuideStep
}

func demoScenarios() []demoScenario {
	return []demoScenario{
		{
			ID: demoScenarioCompany, Kind: "company", Title: "星河科技",
			Summary:  "约 200 人的软件公司：6 个部门的知识库，差旅、CRM、IT 工单 3 个业务系统，带工作台的应用，专家与管理策略，10 位员工。",
			users:    demoUsers,
			kbases:   demoKBases,
			assets:   demoAssets,
			selfApps: demoSelfApps,
			guide:    demoGuide,
		},
		{
			ID: demoScenarioNovel, Kind: "team", Title: "山海工作室",
			Summary:  "4 个人的小说连载团队：设定集和写作规范两个知识库，设定问答与章节进度两个应用，两位审稿专家和一个审稿小组。没有业务系统要接。",
			users:    demoNovelUsers,
			kbases:   demoNovelKBases,
			assets:   demoNovelAssets,
			selfApps: demoNovelSelfApps,
			guide:    demoNovelGuide,
		},
	}
}

func demoScenarioByID(id string) *demoScenario {
	if id == "" {
		id = demoScenarioCompany
	}
	for _, sc := range demoScenarios() {
		if sc.ID == id {
			return &sc
		}
	}
	return nil
}

// demoAllUsers 是所有场景的示例成员。
func demoAllUsers() []demoUser {
	var out []demoUser
	for _, sc := range demoScenarios() {
		out = append(out, sc.users...)
	}
	return out
}

// demoAllGuides 是所有场景的「先试这个」，演示站登录页按场景分组显示。
func demoAllGuides() []demoGuideStep {
	out := []demoGuideStep{}
	for _, sc := range demoScenarios() {
		out = append(out, sc.guide()...)
	}
	return out
}

// demoGuideSteps 把（成员、应用、问题）补全成场景卡片。
func demoGuideSteps(sc string, users []demoUser, steps [][4]string) []demoGuideStep {
	byName := map[string]demoUser{}
	for _, u := range users {
		byName[u.Username] = u
	}
	title := ""
	if s := demoScenarioTitles[sc]; s != "" {
		title = s
	}
	out := make([]demoGuideStep, 0, len(steps))
	for _, st := range steps {
		u := byName[st[0]]
		out = append(out, demoGuideStep{Scenario: sc, ScenarioTitle: title, Username: u.Username, DisplayName: u.DisplayName,
			Department: u.Department, Title: u.Title, App: st[1], AppTitle: st[2], Try: st[3]})
	}
	return out
}

// 场景标识 → 名字（卡片上分组用；和 demoScenarios 里的 Title 一致）
var demoScenarioTitles = map[string]string{demoScenarioCompany: "星河科技", demoScenarioNovel: "山海工作室"}

// demoDepartments 是一个场景里成员所在的部门（去重，按出现顺序）。
func (sc *demoScenario) departments() []string {
	var out []string
	for _, u := range sc.users {
		dup := false
		for _, d := range out {
			dup = dup || d == u.Department
		}
		if !dup {
			out = append(out, u.Department)
		}
	}
	return out
}

// sharedSlugs 是一个场景里对全公司开放的能力（别的场景的人默认也会看到，要靠 demoIsolate 挡开）。
func (sc *demoScenario) sharedSlugs() []string {
	var out []string
	for _, a := range sc.assets() {
		if len(a.Departments) == 0 {
			out = append(out, a.Slug)
		}
	}
	for _, a := range sc.selfApps() {
		if a.Everyone {
			out = append(out, a.Slug)
		}
	}
	return out
}

const demoRecIsolation = "isolation"

// demoIsolate 在两个以上场景同时导入时，给每个场景加一条禁用策略：别的场景的成员用不了它的能力。
// 示例能力的可用范围都是「全公司」（只导入一个场景时管理员自己也能直接试），所以多个场景并存时要靠这个隔开，
// 否则写小说的人会在 DSH 里看到「出差报销助手」。只有一个场景时什么都不做。策略记进 demo_records，清除时一起删。
func (s *Server) demoIsolate(ctx context.Context, by int64) error {
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		return err
	}
	ids := map[string]int64{}
	for _, a := range assets {
		ids[a.Slug] = a.ID
	}
	recs, err := s.store.DemoRecords(ctx)
	if err != nil {
		return err
	}
	ours := map[int64]bool{}
	for _, r := range recs {
		ours[r.RefID] = ours[r.RefID] || r.Kind == "asset"
	}
	type present struct {
		sc    demoScenario
		ids   []int64
		depts []string
	}
	var all []present
	for _, sc := range demoScenarios() {
		p := present{sc: sc, depts: sc.departments()}
		for _, slug := range sc.sharedSlugs() {
			// 只挡导入时建的示例能力，标识撞上用户自己建的不算
			if id, ok := ids[slug]; ok && ours[id] {
				p.ids = append(p.ids, id)
			}
		}
		if len(p.ids) > 0 {
			all = append(all, p)
		}
	}
	if len(all) < 2 {
		return nil
	}
	for _, p := range all {
		var others []string
		for _, q := range all {
			if q.sc.ID != p.sc.ID {
				others = append(others, q.depts...)
			}
		}
		b := &store.Block{}
		for _, r := range recs {
			if r.Kind == demoRecIsolation && r.Slug == p.sc.ID {
				old, err := s.store.GetBlock(ctx, r.RefID)
				if err == nil {
					b = old
				} else if !errors.Is(err, store.ErrNotFound) {
					return err
				}
			}
		}
		created := b.ID == 0
		b.Scope, b.AssetIDs = store.Scope{Departments: others}, p.ids
		b.Note = "示例策略：同时导入了多个示例场景，「" + p.sc.Title + "」的应用、专家和技能只给它自己的成员。"
		if err := s.store.SaveBlock(ctx, b, by); err != nil {
			return err
		}
		if created {
			if err := s.store.RecordDemo(ctx, demoRecIsolation, b.ID, p.sc.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
