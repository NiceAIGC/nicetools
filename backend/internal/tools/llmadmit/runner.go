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
	"time"

	"gorm.io/gorm"

	"nicetools/backend/internal/service"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

// 运行状态机：running → pausing → paused → running → succeeded/failed/aborted。
const (
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

	// active 受通道容量 1 保护：占用即单飞。
	slot   chan struct{}
	cancel context.CancelFunc
}

func NewRunner(db *gorm.DB, svc *service.Service, suites []suite.Suite) *Runner {
	byID := make(map[string]suite.Suite, len(suites))
	for _, s := range suites {
		byID[s.ID] = s
	}
	return &Runner{db: db, svc: svc, suites: byID, slot: make(chan struct{}, 1)}
}

// Recover 在服务重启后把中断的任务标记为 aborted（已完成用例留档不丢）。
func (r *Runner) Recover() error {
	now := time.Now()
	return r.db.Model(&Run{}).
		Where("status IN ?", []string{runRunning, runPausing}).
		Updates(map[string]any{"status": runAborted, "stop_reason": "服务重启，任务中断", "finished_at": &now}).Error
}

// Busy 返回当前是否有活跃任务（含暂停中）；查询异常时不误判，让后续写库暴露真实错误。
func (r *Runner) Busy() (bool, *Run) {
	var run Run
	err := r.db.Where("status IN ?", []string{runRunning, runPausing, runPaused}).Order("created_at DESC").First(&run).Error
	if err != nil {
		return false, nil
	}
	return true, &run
}

func newRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start 创建任务并异步执行；已有活跃任务时返回 409。
func (r *Runner) Start(createdBy string, s suite.Suite, target suite.Target, tier, channel string, declarations map[string]bool) (Run, error) {
	if busy, _ := r.Busy(); busy {
		return Run{}, service.Problem(409, "已有进行中的测试任务，请先完成或终止")
	}
	tierIDs := map[string]bool{}
	var cases []suite.Case
	for _, t := range s.Tiers {
		tierIDs[t.ID] = true
		cases = append(cases, tierCases(t)...)
		if t.ID == tier {
			break
		}
	}
	if !tierIDs[tier] {
		return Run{}, service.Problem(400, "档位无效")
	}
	declJSON, _ := json.Marshal(declarations)
	run := Run{
		ID: newRunID(), CreatedBy: createdBy, Suite: s.ID, SuiteName: s.Name,
		BaseURL: target.BaseURL, APIKey: target.APIKey, Model: target.Model, Channel: channel,
		Tier: tier, Declarations: string(declJSON), Status: runRunning,
		TotalCases: len(cases), CreatedAt: time.Now(),
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		seq := 0
		for _, t := range s.Tiers {
			for _, c := range t.Cases {
				seq++
				row := CaseResult{RunID: run.ID, Seq: seq, CaseID: c.ID, Tier: t.ID, Name: c.Name, Level: string(c.Level), Status: suite.StatusPending, Metrics: "{}", Defects: "[]"}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
			if t.ID == tier {
				break
			}
		}
		actor := createdBy
		if actor == "" {
			actor = "guest"
		}
		return service.Audit(tx, actor, "llmadmit.run.create", fmt.Sprintf("%s %s %s", s.ID, target.Model, tier))
	})
	if err != nil {
		return Run{}, service.DBError(err)
	}
	r.goRun(run.ID)
	return run, nil
}

func tierCases(t suite.Tier) []suite.Case { return t.Cases }

func (r *Runner) goRun(runID string) {
	select {
	case r.slot <- struct{}{}:
	default:
		return // 单飞槽被占（理论上 Start 已挡住，兜底）
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go r.loop(ctx, runID)
}

// Pause 请求暂停：在用例边界生效（长用例内部循环也会检查）。
func (r *Runner) Pause(id string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status = ?", id, runRunning).Update("status", runPausing)
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务不在运行中")
	}
	return nil
}

// Resume 从下一个 pending 用例继续。
func (r *Runner) Resume(id string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status = ?", id, runPaused).Update("status", runRunning)
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务不在暂停中")
	}
	r.goRun(id)
	return nil
}

// Abort 终止任务：立即取消 HTTP，状态落 aborted。
func (r *Runner) Abort(id string, actor string) error {
	res := r.db.Model(&Run{}).Where("id = ? AND status IN ?", id, []string{runRunning, runPausing, runPaused}).
		Updates(map[string]any{"status": runAborted, "stop_reason": "手动终止", "finished_at": time.Now()})
	if res.RowsAffected == 0 {
		return service.Problem(409, "任务已结束")
	}
	if r.cancel != nil {
		r.cancel()
	}
	return service.Audit(r.db, actor, "llmadmit.run.abort", id)
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
	defer func() { <-r.slot }()
	run, err := r.loadRun(runID)
	if err != nil {
		return
	}
	s, ok := r.suites[run.Suite]
	if !ok {
		r.finish(runID, runAborted, "", "套件不存在", nil)
		return
	}
	now := time.Now()
	r.db.Model(&Run{}).Where("id = ?", runID).Update("started_at", &now)
	target := suite.Target{BaseURL: run.BaseURL, APIKey: run.APIKey, Model: run.Model}
	decl := r.declarations(run)

	// 三档顺序执行但不因失败终止：每档产出判定后继续，最终汇总裁决；
	// 仅前置闸门失败（本次测试无效）才中止。
	tierResults := map[string]string{}
	for _, tier := range s.Tiers {
		verdict := r.runTier(ctx, run, tier, target, decl)
		switch verdict {
		case tierPaused:
			r.db.Model(&Run{}).Where("id = ? AND status = ?", runID, runPausing).Update("status", runPaused)
			return
		case tierAborted:
			return
		case tierInvalid:
			r.finish(runID, runFailed, "无效", tier.ID+" 档前置闸门未通过", tierResults)
			return
		}
		tierResults[tier.ID] = verdict
		if tier.ID == run.Tier {
			break
		}
	}
	failedTiers := []string{}
	for id, v := range tierResults {
		if v != tierPassed {
			failedTiers = append(failedTiers, id)
		}
	}
	if len(failedTiers) == 0 {
		r.finish(runID, runSucceeded, "通过", "", tierResults)
		return
	}
	r.finish(runID, runFailed, "不通过", strings.Join(failedTiers, "/")+" 档未通过", tierResults)
}

func (r *Runner) finish(runID, status, verdict, reason string, tierResults map[string]string) {
	now := time.Now()
	results, _ := json.Marshal(tierResults)
	r.db.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
		"status": status, "verdict": verdict, "stop_reason": reason, "tier_results": string(results), "finished_at": &now,
	})
	// 未执行的用例标记跳过，避免界面永远显示「待执行」。
	r.db.Model(&CaseResult{}).Where("run_id = ? AND status IN ?", runID, []string{suite.StatusPending, suite.StatusRunning}).
		Updates(map[string]any{"status": suite.StatusSkipped, "summary": "任务已结束，未执行"})
}

const (
	tierPassed  = "passed"
	tierFailed  = "failed"
	tierPaused  = "paused"
	tierAborted = "aborted"
	tierInvalid = "invalid"
)

func (r *Runner) runTier(ctx context.Context, run Run, tier suite.Tier, target suite.Target, decl map[string]bool) string {
	declared := func(capability string) bool { return decl[capability] }
	failed := false
	p1Count := 0
	for _, c := range tier.Cases {
		switch r.runState(run.ID) {
		case runAborted:
			return tierAborted
		case runPausing:
			return tierPaused
		}
		var row CaseResult
		if err := r.db.Where("run_id = ? AND case_id = ? AND tier = ?", run.ID, c.ID, tier.ID).First(&row).Error; err != nil {
			continue
		}
		if row.Status != suite.StatusPending {
			// 恢复时跳过已完成用例；其缺陷仍计入 P1 统计。
			p1Count += countP1(row.Defects)
			continue
		}
		out := r.execute(ctx, run, c, target, declared, &row)
		if c.Level == suite.LevelGate && out.Status == suite.StatusFailed {
			return tierInvalid
		}
		if out.Status == suite.StatusFailed && (c.Level == suite.LevelP0 || c.Level == suite.LevelRequired) {
			failed = true
		}
		for _, d := range out.Defects {
			switch d.Level {
			case "P0":
				failed = true
			case "P1":
				p1Count++
			}
		}
		// P0 不提前退出：当档剩余用例继续执行，保证数据完整。
	}
	// §1.3：同一档 P1 ≥ 3 → 当档不通过。
	if failed || p1Count >= 3 {
		return tierFailed
	}
	return tierPassed
}

func (r *Runner) runState(runID string) string {
	var status string
	if err := r.db.Model(&Run{}).Where("id = ?", runID).Select("status").Scan(&status).Error; err != nil {
		return runAborted
	}
	return status
}

func countP1(defectsJSON string) int {
	var defects []suite.Defect
	if json.Unmarshal([]byte(defectsJSON), &defects) != nil {
		return 0
	}
	n := 0
	for _, d := range defects {
		if d.Level == "P1" {
			n++
		}
	}
	return n
}

// execute 执行单个用例并落库；被暂停/终止打断的用例回到 pending，恢复时重跑。
func (r *Runner) execute(ctx context.Context, run Run, c suite.Case, target suite.Target, declared func(string) bool, row *CaseResult) suite.Outcome {
	if c.Capability != "" && !declared(c.Capability) {
		return r.persist(run.ID, row, suite.Outcome{Status: suite.StatusExempt, Summary: "能力声明未支持：" + c.Capability})
	}
	now := time.Now()
	r.db.Model(row).Updates(map[string]any{"status": suite.StatusRunning, "started_at": &now})
	caseCtx := &suite.CaseCtx{
		Context: ctx, Target: target, Client: suite.NewClient(),
		Declared:     declared,
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
