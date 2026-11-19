package llmadmit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"nicetools/backend/internal/service"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 运行状态机：running → pausing → paused → running → succeeded/failed/aborted。
const (
	runQueued    = "queued"
	runRunning   = "running"
	runPausing   = "pausing"
	runPaused    = "paused"
	runSucceeded = "succeeded"
	runFailed    = "failed"
	runAborted   = "aborted"
)

var errControlStop = errors.New("run interrupted by control")

// Runner 是全局单飞的异步执行器：同一时间只允许一个活跃任务。
type Runner struct {
	db     *gorm.DB
	svc    *service.Service
	suites map[string]suite.Suite
	origin string // 公网地址（媒体托管 URL 用），空则降级

	// slot 容量 1：同一时间只执行一个任务；其余任务在 queued 状态排队。
	slot       chan struct{}
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	activeID   string
	progressMu sync.Mutex
	progress   map[string]*RunProgress
}

func NewRunner(db *gorm.DB, svc *service.Service, suites []suite.Suite, publicOrigin string) *Runner {
	byID := make(map[string]suite.Suite, len(suites))
	for _, s := range suites {
		byID[s.ID] = s
	}
	return &Runner{db: db, svc: svc, suites: byID, origin: publicOrigin, slot: make(chan struct{}, 1), progress: map[string]*RunProgress{}}
}

// Recover 在服务重启后把中断的任务标记为 aborted（已完成用例留档不丢）。
func (r *Runner) Recover() error {
	now := time.Now()
	if err := r.db.Model(&Run{}).
		Where("status IN ?", []string{runRunning, runPausing}).
		Updates(map[string]any{"status": runAborted, "stop_reason": "服务重启，任务中断", "finished_at": &now}).Error; err != nil {
		return err
	}
	// 重启后继续消费持久化的排队任务。
	r.dispatch()
	return nil
}

func selectedIDs(cases []suite.Case) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	return ids
}

// customCaseIDs 只保存用户显式选的用例（闸门由 SelectIDs 自动补，不写进展示范围）。
func customCaseIDs(cases []suite.Case) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		if c.Group != suite.GroupGate {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func newRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Runner) setProgress(runID string, p RunProgress) {
	if runID == "" {
		return
	}
	now := time.Now()
	r.progressMu.Lock()
	defer r.progressMu.Unlock()
	current := r.progress[runID]
	if current == nil {
		p.StartedAt = now
		current = &p
		r.progress[runID] = current
	} else {
		if p.CaseID != "" {
			current.CaseID, current.CaseName = p.CaseID, p.CaseName
		}
		if p.Step != "" {
			current.Step = p.Step
		}
		current.Done, current.Total = p.Done, p.Total
	}
	current.UpdatedAt = now
}

func (r *Runner) bumpRequest(runID string) {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()
	if p := r.progress[runID]; p != nil {
		p.Requests++
		p.UpdatedAt = time.Now()
	}
}

func (r *Runner) clearProgress(runID string) {
	r.progressMu.Lock()
	delete(r.progress, runID)
	r.progressMu.Unlock()
}

func (r *Runner) progressFor(runID string) *RunProgress {
	r.progressMu.Lock()
	defer r.progressMu.Unlock()
	if p := r.progress[runID]; p != nil {
		copy := *p
		return &copy
	}
	return nil
}

// attachProgress 把实时进度挂到 API 返回的任务副本上。
func (r *Runner) attachProgress(runs []Run) {
	for i := range runs {
		runs[i].Progress = r.progressFor(runs[i].ID)
	}
}

// Start 保留旧调用：不指定自定义用例集，按 preset 选择。
func (r *Runner) Start(createdBy string, s suite.Suite, target suite.Target, preset suite.Group, channel string, declarations map[string]bool, overrides map[string]string) (Run, error) {
	return r.StartWithCases(createdBy, s, target, preset, channel, declarations, overrides, nil)
}

// StartWithCases 创建任务并加入队列；并发任务固定为 1，后续任务显示 queued。
// preset 决定默认跑哪一组用例（空 = 全测）；caseIDs 非空时只跑显式用例并自动带上闸门；
// overrides 为用例级自定义（skip=不测试 / exempt=手动豁免），键不存在的用例或非法取值直接报错。
func (r *Runner) StartWithCases(createdBy string, s suite.Suite, target suite.Target, preset suite.Group, channel string, declarations map[string]bool, overrides map[string]string, caseIDs []string) (Run, error) {
	selected := s.Select(preset)
	if len(caseIDs) > 0 {
		var ok bool
		selected, ok = s.SelectIDs(caseIDs)
		if !ok {
			return Run{}, service.Problem(400, "用例不存在")
		}
	}
	if len(selected) == 0 {
		return Run{}, service.Problem(400, "该预设下没有可执行用例")
	}
	validIDs := make(map[string]bool, len(selected))
	for _, c := range selected {
		validIDs[c.ID] = true
	}
	for id, mode := range overrides {
		if !validIDs[id] {
			return Run{}, service.Problem(400, "用例不存在: "+id)
		}
		if mode != "skip" && mode != "exempt" {
			return Run{}, service.Problem(400, "用例覆盖取值仅支持 skip/exempt")
		}
	}
	// 合并管理员永久配置：mode 作为默认（任务级覆盖优先），level 直接改写用例级别。
	var cfgs []CaseCfg
	if err := r.db.Where("suite = ?", s.ID).Find(&cfgs).Error; err != nil {
		return Run{}, err
	}
	levelOf := map[string]string{}
	merged := map[string]string{}
	for _, cfg := range cfgs {
		if cfg.Mode != "" {
			merged[cfg.CaseID] = cfg.Mode
		}
		if cfg.Level != "" {
			levelOf[cfg.CaseID] = cfg.Level
		}
	}
	for id, mode := range overrides {
		merged[id] = mode
	}
	// 未显式指定 case_ids 的常规整组测试默认跳过可选/昂贵用例；
	// 显式选择时不受影响，管理员已配置的 skip/exempt 也优先保留。
	if len(caseIDs) == 0 {
		for _, c := range selected {
			if c.DefaultSkip {
				if _, exists := merged[c.ID]; !exists {
					merged[c.ID] = "skip"
				}
			}
		}
	}
	// 显式勾选用例时，勾选即视为要求验证该能力，自动声明对应能力。
	if len(caseIDs) > 0 {
		for _, c := range selected {
			if c.Capability != "" {
				declarations[c.Capability] = true
			}
		}
	}
	declJSON, _ := json.Marshal(declarations)
	ovrJSON, _ := json.Marshal(merged)
	customIDs := ""
	if len(caseIDs) > 0 {
		customIDs = strings.Join(customCaseIDs(selected), ",")
		if customIDs == "" {
			customIDs = strings.Join(selectedIDs(selected), ",")
		}
	}
	run := Run{
		ID: newRunID(), CreatedBy: createdBy, Suite: s.ID, SuiteName: s.Name,
		BaseURL: target.BaseURL, PrimaryDomain: primaryDomain(target.BaseURL), APIKey: target.APIKey, Model: target.Model, Channel: channel,
		Preset: string(preset), Declarations: string(declJSON), CaseOverrides: string(ovrJSON), CaseIDs: customIDs, Status: runQueued,
		MediaToken: newRunID(), TotalCases: len(selected), CreatedAt: time.Now(),
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		for seq, c := range selected {
			level := string(c.Level)
			if override := levelOf[c.ID]; override != "" {
				level = override
			}
			row := CaseResult{RunID: run.ID, Seq: seq + 1, CaseID: c.ID, Group: c.Group, Name: c.Name, Level: level, Status: suite.StatusPending, Metrics: "{}", Defects: "[]"}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		actor := createdBy
		if actor == "" {
			actor = "guest"
		}
		return service.Audit(tx, actor, "llmadmit.run.create", fmt.Sprintf("%s %s %s", s.ID, target.Model, preset))
	})
	if err != nil {
		return Run{}, service.DBError(err)
	}
	r.dispatch()
	return run, nil
}

func (r *Runner) launch(runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.mu.Lock()
	r.cancel, r.done, r.activeID = cancel, done, runID
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			if r.activeID == runID {
				r.activeID = ""
			}
			r.mu.Unlock()
			close(done)
		}()
		r.loop(ctx, runID)
	}()
}

// dispatch 从 queued 队列取最早任务启动；并发容量固定为 1。
func (r *Runner) dispatch() {
	select {
	case r.slot <- struct{}{}:
	default:
		return
	}
	// 暂停任务已经释放执行槽，不再阻塞 queued；槽位本身保证同一时间只跑一个任务。
	var run Run
	if err := r.db.Where("status = ?", runQueued).Order("created_at ASC, id ASC").First(&run).Error; err != nil {
		<-r.slot
		return
	}
	now := time.Now()
	if err := r.db.Model(&Run{}).Where("id = ? AND status = ?", run.ID, runQueued).
		Updates(map[string]any{"status": runRunning, "started_at": &now}).Error; err != nil {
		<-r.slot
		return
	}
	r.launch(run.ID)
}

// Pause 请求暂停：在用例边界生效（长用例内部循环也会检查）。
func (r *Runner) Pause(id string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status = ?", id, runRunning).Update("status", runPausing)
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务不在运行中")
	}
	// 暂停时立即取消当前请求，避免长 HTTP/30 分钟超时让任务长时间停在 pausing。
	r.mu.Lock()
	activeID, cancel := r.activeID, r.cancel
	r.mu.Unlock()
	if activeID == id && cancel != nil {
		cancel()
	}
	return nil
}

// Resume 从下一个 pending 用例继续；支持暂停和已终止任务。
// 恢复只入队，不抢占正在执行的任务；dispatch 在槽位空出后按创建顺序启动。
func (r *Runner) Resume(id string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status IN ?", id, []string{runPaused, runAborted}).
		Updates(map[string]any{"status": runQueued, "finished_at": nil, "stop_reason": "", "verdict": ""})
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务不在暂停或已终止状态")
	}
	// 终止时当前用例可能仍处于 running；恢复前回到 pending 重跑。
	r.db.Model(&CaseResult{}).Where("run_id = ? AND status = ?", id, suite.StatusRunning).
		Updates(map[string]any{"status": suite.StatusPending, "started_at": nil})
	r.dispatch()
	return nil
}

// Abort 终止任务：立即取消 HTTP，状态落 aborted。
func (r *Runner) Abort(id string, actor string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status IN ?", id, []string{runQueued, runRunning, runPausing, runPaused}).
		Updates(map[string]any{"status": runAborted, "stop_reason": "手动终止", "finished_at": time.Now()})
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务已结束")
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// 等旧循环释放单飞槽，避免用户很快点“继续”时启动两个循环。
	if done != nil {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
		}
	}
	_ = service.Audit(r.db, actor, "llmadmit.run.abort", id)
	r.dispatch()
	return nil
}

func (r *Runner) loadRun(id string) (Run, error) {
	var run Run
	return run, r.db.First(&run, "id = ?", id).Error
}

func (r *Runner) declarations(run Run) map[string]bool {
	out := map[string]bool{}
	_ = json.Unmarshal([]byte(run.Declarations), &out)
	return out
}

func (r *Runner) overrides(run Run) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal([]byte(run.CaseOverrides), &out)
	return out
}

// control 返回长用例内部循环的控制检查函数。
func (r *Runner) control(ctx context.Context, runID string) func() error {
	return func() error {
		if ctx.Err() != nil {
			return errControlStop
		}
		var status string
		if err := r.db.Model(&Run{}).Where("id = ?", runID).Select("status").Scan(&status).Error; err != nil {
			return err
		}
		if status == runPausing || status == runAborted {
			return errControlStop
		}
		return nil
	}
}

func (r *Runner) loop(ctx context.Context, runID string) {
	defer func() {
		r.clearProgress(runID)
		<-r.slot
		r.dispatch()
	}()
	run, err := r.loadRun(runID)
	if err != nil {
		return
	}
	s, ok := r.suites[run.Suite]
	if !ok {
		r.finish(runID, runAborted, "", "套件不存在")
		return
	}
	now := time.Now()
	r.db.Model(&Run{}).Where("id = ?", runID).Update("started_at", &now)
	target := suite.Target{BaseURL: run.BaseURL, APIKey: run.APIKey, Model: run.Model}

	selected := s.Select(suite.Group(run.Preset))
	if run.CaseIDs != "" {
		if ids, ok := s.SelectIDs(strings.Split(run.CaseIDs, ",")); ok {
			selected = ids
		}
	}
	switch r.runAll(ctx, run, selected, target, r.declarations(run), r.overrides(run)) {
	case progressPaused:
		r.markPaused(runID)
		return
	case progressAborted:
		return
	case progressInvalid:
		// 闸门不过 → 本次测试无效，不产出 P0/P1 结论。
		r.finish(runID, runFailed, "无效", "前置闸门未通过，本次测试不作数")
		return
	}
	// 裁决不写「是否通过」，按级别统计未通过项（P0×n、P1×n …）。
	var rows []CaseResult
	r.db.Where("run_id = ?", runID).Find(&rows)
	total := map[string]int{}
	for _, row := range rows {
		if row.Status == suite.StatusFailed {
			total[normalizeLevel(row.Level)]++
		}
	}
	verdict := verdictText(total)
	status := runSucceeded
	if verdict != "0 项未通过" {
		status = runFailed
	}
	// 未通过原因已由 verdict 与缺陷清单表达，不再重复写入说明行。
	r.finish(runID, status, verdict, "")
}

// markPaused 持久化暂停终态；短暂 SQLite 锁冲突时重试，避免任务永远停在 pausing。
func (r *Runner) markPaused(runID string) {
	for attempt := 0; attempt < 5; attempt++ {
		res := r.db.Model(&Run{}).Where("id = ? AND status = ?", runID, runPausing).Update("status", runPaused)
		if res.Error == nil && res.RowsAffected > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// runAll 的收尾状态：除正常跑完外，暂停/终止/无效都需要中断整个任务。
const (
	progressDone    = "done"
	progressPaused  = "paused"
	progressAborted = "aborted"
	progressInvalid = "invalid"
)

// runAll 顺序执行选中用例；P0 失败不提前退出（保证数据完整），仅闸门失败判定整次无效。
func (r *Runner) runAll(ctx context.Context, run Run, cases []suite.Case, target suite.Target, decl map[string]bool, ovr map[string]string) string {
	declared := func(capability string) bool { return decl[capability] }
	for _, c := range cases {
		if ctx.Err() != nil {
			switch r.runState(run.ID) {
			case runPausing:
				return progressPaused
			default:
				return progressAborted
			}
		}
		switch r.runState(run.ID) {
		case runAborted:
			return progressAborted
		case runPausing:
			return progressPaused
		}
		var row CaseResult
		if err := r.db.Where("run_id = ? AND case_id = ?", run.ID, c.ID).First(&row).Error; err != nil {
			continue
		}
		if row.Status != suite.StatusPending {
			continue // 恢复时跳过已完成用例
		}
		// 任务级自定义：skip=不测试，exempt=手动豁免（均不影响裁决）。
		switch ovr[c.ID] {
		case "skip":
			r.persist(run.ID, &row, suite.Outcome{Status: suite.StatusSkipped, Summary: "手动选择不测试"})
			continue
		case "exempt":
			r.persist(run.ID, &row, suite.Outcome{Status: suite.StatusExempt, Summary: "手动跳过"})
			continue
		}
		if out := r.execute(ctx, run, c, target, declared, &row); c.Level == suite.LevelGate && out.Status == suite.StatusFailed {
			return progressInvalid
		}
	}
	return progressDone
}

func (r *Runner) finish(runID, status, verdict, reason string) {
	now := time.Now()
	r.db.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
		"status": status, "verdict": verdict, "stop_reason": reason, "finished_at": &now,
	})
	// 未执行的用例标记跳过，避免界面永远显示「待执行」。
	r.db.Model(&CaseResult{}).Where("run_id = ? AND status IN ?", runID, []string{suite.StatusPending, suite.StatusRunning}).
		Updates(map[string]any{"status": suite.StatusSkipped, "summary": "任务已结束，未执行"})
}

func (r *Runner) runState(runID string) string {
	var status string
	if err := r.db.Model(&Run{}).Where("id = ?", runID).Select("status").Scan(&status).Error; err != nil {
		return runAborted
	}
	return status
}

// execute 执行单个用例并落库；被暂停/终止打断的用例回到 pending，恢复时重跑。
func (r *Runner) execute(ctx context.Context, run Run, c suite.Case, target suite.Target, declared func(string) bool, row *CaseResult) suite.Outcome {
	if c.Capability != "" && !declared(c.Capability) {
		return r.persist(run.ID, row, suite.Outcome{Status: suite.StatusExempt, Summary: "能力声明未支持：" + c.Capability})
	}
	now := time.Now()
	r.db.Model(row).Updates(map[string]any{"status": suite.StatusRunning, "started_at": &now})
	client := suite.NewClient()
	client.OnRequest = func() { r.bumpRequest(run.ID) }
	r.setProgress(run.ID, RunProgress{CaseID: c.ID, CaseName: c.Name, StartedAt: now})
	caseCtx := &suite.CaseCtx{
		Context: ctx, Target: target, Client: client,
		Declared:  declared,
		OnRequest: func() { r.bumpRequest(run.ID) },
		Progress: func(done, total int, step string) {
			r.setProgress(run.ID, RunProgress{CaseID: c.ID, CaseName: c.Name, Step: step, Done: done, Total: total})
		},
		MediaURL: func(name string) string {
			if r.origin == "" || run.MediaToken == "" {
				return ""
			}
			return fmt.Sprintf("%s/api/tools/llm-admission/pub/media/%s/%s", r.origin, run.MediaToken, name)
		},
		CheckControl: r.control(ctx, run.ID),
	}
	var out suite.Outcome
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				out = suite.Outcome{Status: suite.StatusFailed, Summary: fmt.Sprintf("用例内部错误: %v", rec)}
			}
		}()
		out = c.Run(caseCtx)
	}()
	if ctx.Err() != nil {
		// 旧循环已取消（暂停/终止/重启）：即使任务随后被“继续”，也不能写终态。
		r.db.Model(row).Updates(map[string]any{"status": suite.StatusPending, "started_at": nil})
		return out
	}
	if state := r.runState(run.ID); state == runPausing || state == runAborted {
		// 暂停/终止：用例回到 pending，恢复后重跑；已完成用例进度不回退。
		r.db.Model(row).Updates(map[string]any{"status": suite.StatusPending, "started_at": nil})
		return out
	}
	return r.persist(run.ID, row, out)
}

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (r *Runner) persist(runID string, row *CaseResult, out suite.Outcome) suite.Outcome {
	if out.Status == "" || out.Status == suite.StatusRunning {
		out.Status = suite.StatusFailed
		out.Summary = "用例未产出结论"
	}
	if out.Metrics == nil {
		out.Metrics = map[string]any{}
	}
	if out.Defects == nil {
		out.Defects = []suite.Defect{}
	}
	metrics, _ := json.Marshal(out.Metrics)
	defects, _ := json.Marshal(out.Defects)
	evidence, _ := json.Marshal(out.Evidence)
	sha := sha256Of(string(evidence))
	finished := time.Now()
	updates := map[string]any{
		"status": out.Status, "summary": out.Summary, "metrics": string(metrics),
		"defects": string(defects), "evidence": string(evidence), "evidence_sha": sha,
		"finished_at": &finished,
	}
	// 汇总首条证据的传输层信息到列，便于列表与报告检索。
	for _, ev := range out.Evidence {
		if updates["http_status"] == nil && ev.Result.HTTPStatus != 0 {
			updates["http_status"] = ev.Result.HTTPStatus
		}
		if updates["request_id"] == nil && ev.Result.RequestID != "" {
			updates["request_id"] = ev.Result.RequestID
		}
		updates["retries"] = row.Retries + ev.Retries
	}
	if row.StartedAt != nil {
		updates["duration_ms"] = finished.Sub(*row.StartedAt).Milliseconds()
	}
	if err := r.db.Model(row).Updates(updates).Error; err == nil {
		r.db.Model(&Run{}).Where("id = ?", runID).Update("done_cases", gorm.Expr("done_cases + 1"))
	}
	return out
}
