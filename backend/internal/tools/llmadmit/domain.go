package llmadmit

import (
	"net"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
)

// primaryDomain 把网关地址归一到主域名：api.foo.com -> foo.com，www.foo.com -> foo.com。
// IP、localhost 和无法解析的主机保留 host[:port]，避免把不同入口误合并。
func primaryDomain(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return strings.TrimSpace(raw)
	}
	if net.ParseIP(host) != nil {
		if port := u.Port(); port != "" {
			return net.JoinHostPort(host, port)
		}
		return host
	}
	if base, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		host = base
	}
	if port := u.Port(); port != "" {
		return host + ":" + port
	}
	return host
}

func fullHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return primaryDomain(raw)
	}
	return strings.ToLower(u.Host)
}

// backfillPrimaryDomains 给升级前创建的任务补主域名；失败不影响启动。
func backfillPrimaryDomains(db *gorm.DB) error {
	var runs []Run
	if err := db.Where("primary_domain = ? OR primary_domain IS NULL", "").Find(&runs).Error; err != nil {
		return err
	}
	for i := range runs {
		domain := primaryDomain(runs[i].BaseURL)
		if domain == "" {
			continue
		}
		if err := db.Model(&Run{}).Where("id = ?", runs[i].ID).Update("primary_domain", domain).Error; err != nil {
			return err
		}
	}
	return nil
}

type domainGroup struct {
	Domain        string         `json:"domain"`
	Note          string         `json:"note"`
	Name          string         `json:"name"`
	Hosts         []string       `json:"hosts"`
	Models        []string       `json:"models"`
	Channels      []string       `json:"channels"`
	RunCount      int            `json:"run_count"`
	ActiveCount   int            `json:"active_count"`
	Latest        *Run           `json:"latest,omitempty"`
	LastCompleted *Run           `json:"last_completed,omitempty"`
	FailedByLevel map[string]int `json:"failed_by_level"`
	Runs          []Run          `json:"runs"`
}

// annotateRuns 批量计算任务通过率/评级；只读取轻量列，避免把证据原文全部载入。
func (m *Module) annotateRuns(runs []Run) {
	if len(runs) == 0 {
		return
	}
	ids := make([]string, 0, len(runs))
	for i := range runs {
		ids = append(ids, runs[i].ID)
	}
	var rows []CaseResult
	_ = m.db.Model(&CaseResult{}).Select("run_id,status,level").Where("run_id IN ?", ids).Find(&rows).Error
	byRun := make(map[string][]CaseResult, len(runs))
	for _, row := range rows {
		byRun[row.RunID] = append(byRun[row.RunID], row)
	}
	for i := range runs {
		scoreRun(&runs[i], byRun[runs[i].ID])
	}
}

// runsByDomain 聚合运行记录；所有历史任务都在响应中，前端只按域名折叠展示。
func (m *Module) runsByDomain() ([]domainGroup, error) {
	var runs []Run
	if err := m.db.Order("created_at DESC").Limit(5000).Find(&runs).Error; err != nil {
		return nil, err
	}
	m.annotateRuns(runs)
	if m.runner != nil {
		m.runner.attachProgress(runs)
	}
	var notes []DomainNote
	if err := m.db.Find(&notes).Error; err != nil {
		return nil, err
	}
	noteByDomain := make(map[string]string, len(notes))
	for _, note := range notes {
		noteByDomain[note.Domain] = note.Note
	}
	groups := make(map[string]*domainGroup)
	hostSeen := make(map[string]map[string]bool)
	modelSeen := make(map[string]map[string]bool)
	channelSeen := make(map[string]map[string]bool)
	for i := range runs {
		run := runs[i]
		domain := run.PrimaryDomain
		if domain == "" {
			domain = primaryDomain(run.BaseURL)
		}
		if domain == "" {
			domain = "未知域名"
		}
		g := groups[domain]
		if g == nil {
			g = &domainGroup{Domain: domain, Note: noteByDomain[domain], FailedByLevel: map[string]int{}}
			groups[domain] = g
			hostSeen[domain] = map[string]bool{}
			modelSeen[domain] = map[string]bool{}
			channelSeen[domain] = map[string]bool{}
		}
		g.Runs = append(g.Runs, run)
		g.RunCount++
		switch run.Status {
		case runQueued, runRunning, runPausing, runPaused:
			g.ActiveCount++
		}
		if g.Latest == nil || run.CreatedAt.After(g.Latest.CreatedAt) {
			copy := run
			g.Latest = &copy
			if strings.TrimSpace(run.Channel) != "" {
				g.Name = run.Channel
			}
		}
		terminal := run.Status == runSucceeded || run.Status == runFailed || run.Status == runAborted
		if terminal && (g.LastCompleted == nil || (run.FinishedAt != nil && (g.LastCompleted.FinishedAt == nil || run.FinishedAt.After(*g.LastCompleted.FinishedAt)))) {
			copy := run
			g.LastCompleted = &copy
		}
		host := fullHost(run.BaseURL)
		if host != "" && !hostSeen[domain][host] {
			hostSeen[domain][host] = true
			g.Hosts = append(g.Hosts, host)
		}
		if run.Model != "" && !modelSeen[domain][run.Model] {
			modelSeen[domain][run.Model] = true
			g.Models = append(g.Models, run.Model)
		}
		if run.Channel != "" && !channelSeen[domain][run.Channel] {
			channelSeen[domain][run.Channel] = true
			g.Channels = append(g.Channels, run.Channel)
		}
	}
	out := make([]domainGroup, 0, len(groups))
	for _, g := range groups {
		if g.Name == "" {
			g.Name = g.Domain
		}
		sort.Strings(g.Hosts)
		sort.Strings(g.Models)
		sort.Strings(g.Channels)
		if g.LastCompleted != nil {
			var rows []CaseResult
			_ = m.db.Model(&CaseResult{}).Select("level").Where("run_id = ? AND status = ?", g.LastCompleted.ID, "failed").Find(&rows).Error
			for _, row := range rows {
				if level := normalizeLevel(row.Level); level != "" {
					g.FailedByLevel[level]++
				}
			}
		}
		sort.Slice(g.Runs, func(i, j int) bool { return g.Runs[i].CreatedAt.After(g.Runs[j].CreatedAt) })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].Runs[0].CreatedAt, out[j].Runs[0].CreatedAt
		if out[i].Latest != nil {
			ti = out[i].Latest.CreatedAt
		}
		if out[j].Latest != nil {
			tj = out[j].Latest.CreatedAt
		}
		return ti.After(tj)
	})
	return out, nil
}
