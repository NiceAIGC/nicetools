// Package llmadmit 是大模型准入测试工具的后端模块。
// 测试在服务端异步执行：单飞、可暂停/继续、报告落库，导出分详细/脱敏两版。
package llmadmit

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nicetools/backend/internal/module"
	"nicetools/backend/internal/service"
	"nicetools/backend/internal/tools/llmadmit/k3"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

// Suites 是准入标准注册表；新增标准在此加一行。
func Suites() []suite.Suite {
	return []suite.Suite{k3.Suite()}
}

type Module struct {
	runner *Runner
	db     *gorm.DB
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return "llm-admission" }

func (m *Module) Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Run{}, &CaseResult{})
}

func (m *Module) Register(read, write *gin.RouterGroup, d module.Deps) {
	m.db = d.DB
	m.runner = NewRunner(d.DB, d.Service, Suites())
	// 重启恢复：中断任务标记 aborted，已完成用例留档不丢。
	if err := m.runner.Recover(); err != nil {
		panic(err)
	}

	read.GET("/suites", m.suites)
	read.GET("/runs", m.runs)
	read.GET("/runs/:rid", m.runDetail)
	read.GET("/runs/:rid/export", m.export)
	read.GET("/export/merged", m.exportMerged)
	write.POST("/runs", m.create)
	write.POST("/runs/:rid/pause", func(c *gin.Context) { m.control(c, m.runner.Pause) })
	write.POST("/runs/:rid/resume", func(c *gin.Context) { m.control(c, m.runner.Resume) })
	write.POST("/runs/:rid/abort", m.abort)
}

func (m *Module) suites(c *gin.Context) {
	c.JSON(200, Suites())
}

func (m *Module) runs(c *gin.Context) {
	var list []Run
	if err := m.db.Order("created_at DESC").Limit(100).Find(&list).Error; err != nil {
		module.Fail(c, err)
		return
	}
	if list == nil {
		list = []Run{}
	}
	c.JSON(200, list)
}

func (m *Module) findRun(c *gin.Context) (Run, bool) {
	var run Run
	if err := m.db.First(&run, "id = ?", c.Param("rid")).Error; err != nil {
		module.Fail(c, service.DBError(err))
		return run, false
	}
	return run, true
}

// runDetail 返回任务与用例列表；证据原文只随导出下发，列表不含。
func (m *Module) runDetail(c *gin.Context) {
	run, ok := m.findRun(c)
	if !ok {
		return
	}
	var rows []CaseResult
	if err := m.db.Where("run_id = ?", run.ID).Order("seq ASC").Find(&rows).Error; err != nil {
		module.Fail(c, err)
		return
	}
	for i := range rows {
		rows[i].Evidence = ""
	}
	c.JSON(200, gin.H{"run": run, "cases": rows})
}

type createInput struct {
	BaseURL      string          `json:"base_url"`
	APIKey       string          `json:"api_key"`
	Model        string          `json:"model"`
	Channel      string          `json:"channel"`
	Suite        string          `json:"suite"`
	Tier         string          `json:"tier"`
	Declarations map[string]bool `json:"declarations"`
}

func (m *Module) create(c *gin.Context) {
	var input createInput
	if err := c.ShouldBindJSON(&input); err != nil {
		module.Fail(c, service.Problem(400, "请求 JSON 无效"))
		return
	}
	input.BaseURL = strings.TrimSuffix(strings.TrimSpace(input.BaseURL), "/")
	u, err := url.Parse(input.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		module.Fail(c, service.Problem(400, "网关地址须为 http(s) URL"))
		return
	}
	if len(input.APIKey) == 0 || len(input.APIKey) > 256 {
		module.Fail(c, service.Problem(400, "密钥长度须为 1–256"))
		return
	}
	var chosen suite.Suite
	found := false
	for _, s := range Suites() {
		if s.ID == input.Suite {
			chosen, found = s, true
			break
		}
	}
	if !found {
		module.Fail(c, service.Problem(400, "测试标准不存在"))
		return
	}
	if strings.TrimSpace(input.Model) == "" {
		input.Model = chosen.DefaultModel
	}
	if !service.Bounded(input.Model, 64) || !service.Bounded(input.Channel, 64) {
		module.Fail(c, service.Problem(400, "模型或渠道名称过长"))
		return
	}
	input.Channel = strings.TrimSpace(input.Channel)
	// 只接受套件声明过的能力键，忽略前端夹带。
	declarations := map[string]bool{}
	for _, cap := range chosen.Capabilities {
		declarations[cap.Key] = input.Declarations[cap.Key]
	}
	actor := "guest"
	if user := module.CurrentUser(c); user != nil {
		actor = user.Username
	}
	run, err := m.runner.Start(actor, chosen, suite.Target{BaseURL: input.BaseURL, APIKey: input.APIKey, Model: input.Model}, input.Tier, input.Channel, declarations)
	if err != nil {
		module.Fail(c, err)
		return
	}
	c.JSON(201, run)
}

func (m *Module) control(c *gin.Context, action func(string) error) {
	if _, ok := m.findRun(c); !ok {
		return
	}
	if err := action(c.Param("rid")); err != nil {
		module.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (m *Module) abort(c *gin.Context) {
	if _, ok := m.findRun(c); !ok {
		return
	}
	actor := "guest"
	if user := module.CurrentUser(c); user != nil {
		actor = user.Username
	}
	if err := m.runner.Abort(c.Param("rid"), actor); err != nil {
		module.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// export 导出报告：type=full|masked，format=json|md，channel 可覆盖渠道名；
// 两版都带 niceaigc 水印。
func (m *Module) export(c *gin.Context) {
	run, ok := m.findRun(c)
	if !ok {
		return
	}
	var rows []CaseResult
	if err := m.db.Where("run_id = ?", run.ID).Order("seq ASC").Find(&rows).Error; err != nil {
		module.Fail(c, err)
		return
	}
	rep := m.buildReport(run, rows, c.DefaultQuery("type", "full") == "masked", c.Query("channel"))
	m.respondReport(c, rep, "llmadmit-"+run.ID)
}

// exportMerged 合并多个任务导出：ids= id 逗号分隔，同用例取最新结果。
func (m *Module) exportMerged(c *gin.Context) {
	ids := strings.Split(c.Query("ids"), ",")
	if len(ids) == 0 || ids[0] == "" || len(ids) > 20 {
		module.Fail(c, service.Problem(400, "ids 须为 1–20 个任务 id"))
		return
	}
	var runs []Run
	if err := m.db.Where("id IN ?", ids).Order("created_at ASC").Find(&runs).Error; err != nil {
		module.Fail(c, err)
		return
	}
	if len(runs) != len(ids) {
		module.Fail(c, service.Problem(404, "部分任务不存在"))
		return
	}
	rowsByID := map[string][]CaseResult{}
	for _, run := range runs {
		var rows []CaseResult
		if err := m.db.Where("run_id = ?", run.ID).Order("seq ASC").Find(&rows).Error; err != nil {
			module.Fail(c, err)
			return
		}
		rowsByID[run.ID] = rows
	}
	rep := m.buildMergedReport(runs, rowsByID, c.DefaultQuery("type", "full") == "masked", c.Query("channel"))
	m.respondReport(c, rep, "llmadmit-merged")
}

func (m *Module) respondReport(c *gin.Context, rep report, base string) {
	kind := "full"
	if rep.Masked {
		kind = "masked"
	}
	filename := base + "-" + kind
	switch c.DefaultQuery("format", "json") {
	case "json":
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			module.Fail(c, err)
			return
		}
		c.Header("Content-Disposition", "attachment; filename=\""+filename+".json\"")
		c.Data(200, "application/json; charset=utf-8", data)
	case "md":
		c.Header("Content-Disposition", "attachment; filename=\""+filename+".md\"")
		c.Data(200, "text/markdown; charset=utf-8", []byte(renderMarkdown(rep)))
	default:
		module.Fail(c, service.Problem(400, "format 仅支持 json/md"))
	}
}

// 编译期契约校验。
var _ module.Module = (*Module)(nil)
