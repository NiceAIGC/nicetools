package llmadmit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"nicetools/backend/internal/service"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

func attemptFromCase(runID, caseID, source string, no int, row CaseResult) CaseAttempt {
	created := time.Now()
	if row.FinishedAt != nil {
		created = *row.FinishedAt
	} else if row.StartedAt != nil {
		created = *row.StartedAt
	}
	return CaseAttempt{
		RunID: runID, CaseID: caseID, AttemptNo: no, Source: source,
		Status: row.Status, HTTPStatus: row.HTTPStatus, DurationMs: row.DurationMs,
		Retries: row.Retries, RequestID: row.RequestID, Summary: row.Summary,
		Metrics: row.Metrics, Defects: row.Defects, Evidence: row.Evidence,
		EvidenceSHA: row.EvidenceSHA, CreatedAt: created,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
}

// snapshotCase 在第一次复测前把当前结果保存为 attempt 0，保证旧结果可导出。
func (r *Runner) snapshotCase(runID, caseID string) error {
	var count int64
	if err := r.db.Model(&CaseAttempt{}).Where("run_id = ? AND case_id = ?", runID, caseID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var row CaseResult
	if err := r.db.Where("run_id = ? AND case_id = ?", runID, caseID).First(&row).Error; err != nil {
		return err
	}
	attempt := attemptFromCase(runID, caseID, "original", 0, row)
	return r.db.Create(&attempt).Error
}

// StartCaseRecheck 创建并异步执行一个细则复测；不创建新的顶层 Run。
func (r *Runner) StartCaseRecheck(runID, caseID string, includeGates bool) (CaseAttempt, error) {
	run, err := r.loadRun(runID)
	if err != nil {
		return CaseAttempt{}, service.Problem(404, "任务不存在")
	}
	switch run.Status {
	case runSucceeded, runFailed, runAborted:
	default:
		return CaseAttempt{}, service.Problem(409, "任务结束后才能复测细则")
	}
	s, ok := r.suites[run.Suite]
	if !ok {
		return CaseAttempt{}, service.Problem(404, "测试标准不存在")
	}
	var target suite.Case
	found := false
	for _, c := range s.Cases {
		if c.ID == caseID {
			target, found = c, true
			break
		}
	}
	if !found {
		return CaseAttempt{}, service.Problem(404, "细则不存在")
	}
	var active int64
	if err := r.db.Model(&CaseAttempt{}).Where("run_id = ? AND case_id = ? AND status IN ?", runID, caseID, []string{"queued", "running"}).Count(&active).Error; err != nil {
		return CaseAttempt{}, err
	}
	if active > 0 {
		return CaseAttempt{}, service.Problem(409, "该细则正在复测")
	}
	if err := r.snapshotCase(runID, caseID); err != nil {
		return CaseAttempt{}, err
	}
	var maxNo int
	var last CaseAttempt
	if err := r.db.Where("run_id = ? AND case_id = ?", runID, caseID).Order("attempt_no DESC").First(&last).Error; err == nil {
		maxNo = last.AttemptNo
	}
	attempt := CaseAttempt{
		RunID: runID, CaseID: caseID, AttemptNo: maxNo + 1, Source: "recheck",
		Status: "queued", CreatedAt: time.Now(), Summary: "等待执行",
	}
	if err := r.db.Create(&attempt).Error; err != nil {
		return CaseAttempt{}, service.Problem(500, "创建复测记录失败")
	}
	go r.runCaseAttempt(runID, caseID, target, attempt.ID, includeGates)
	return attempt, nil
}

func (r *Runner) runCaseAttempt(runID, caseID string, target suite.Case, attemptID uint, includeGates bool) {
	// 复用单飞槽：复测不会和完整任务同时压被测网关。
	r.slot <- struct{}{}
	defer func() {
		r.clearProgress(runID)
		<-r.slot
		r.dispatch()
	}()

	var attempt CaseAttempt
	if err := r.db.First(&attempt, attemptID).Error; err != nil {
		return
	}
	run, err := r.loadRun(runID)
	if err != nil {
		r.finishAttempt(attempt, suite.Outcome{Status: suite.StatusFailed, Summary: "任务不存在"})
		return
	}
	start := time.Now()
	attempt.Status, attempt.StartedAt = "running", &start
	r.db.Save(&attempt)
	r.setProgress(runID, RunProgress{CaseID: caseID, CaseName: target.Name, Step: "复测", StartedAt: start})

	client := suite.NewClient()
	client.OnRequest = func() { r.bumpRequest(runID) }
	ctx := context.Background()
	caseCtx := &suite.CaseCtx{
		Context: ctx, Target: suite.Target{BaseURL: run.BaseURL, APIKey: run.APIKey, Model: run.Model}, Client: client,
		Declared: func(string) bool { return true },
		MediaURL: func(name string) string {
			if r.origin == "" || run.MediaToken == "" {
				return ""
			}
			return fmt.Sprintf("%s/api/tools/llm-admission/pub/media/%s/%s", r.origin, run.MediaToken, name)
		},
		CheckControl: func() error { return nil },
		OnRequest:    func() { r.bumpRequest(runID) },
		Progress: func(done, total int, step string) {
			r.setProgress(runID, RunProgress{CaseID: caseID, CaseName: target.Name, Step: step, Done: done, Total: total})
		},
	}
	if includeGates {
		s := r.suites[run.Suite]
		for _, gate := range s.Cases {
			if gate.Group != suite.GroupGate {
				continue
			}
			out := gate.Run(caseCtx)
			if out.Status != suite.StatusPassed {
				attempt.Status = suite.StatusFailed
				attempt.Summary = "前置闸门未通过: " + gate.ID + " " + out.Summary
				metrics, _ := json.Marshal(out.Metrics)
				defects, _ := json.Marshal(out.Defects)
				evidence, _ := json.Marshal(out.Evidence)
				attempt.Metrics, attempt.Defects, attempt.Evidence = string(metrics), string(defects), string(evidence)
				attempt.EvidenceSHA = sha256Of(string(evidence))
				finished := time.Now()
				attempt.FinishedAt = &finished
				attempt.DurationMs = finished.Sub(start).Milliseconds()
				attempt.AttemptNo = 0 // not applied to current result
				r.db.Save(&attempt)
				return
			}
		}
	}

	var out suite.Outcome
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				out = suite.Outcome{Status: suite.StatusFailed, Summary: fmt.Sprintf("复测内部错误: %v", rec)}
			}
		}()
		out = target.Run(caseCtx)
	}()
	r.finishAttempt(attempt, out)
}

func (r *Runner) finishAttempt(attempt CaseAttempt, out suite.Outcome) {
	if out.Status == "" || out.Status == suite.StatusRunning {
		out.Status = suite.StatusFailed
		out.Summary = "复测未产出结论"
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
	now := time.Now()
	attempt.Status, attempt.Summary = out.Status, out.Summary
	attempt.Metrics, attempt.Defects, attempt.Evidence = string(metrics), string(defects), string(evidence)
	attempt.EvidenceSHA = sha256Of(string(evidence))
	attempt.FinishedAt = &now
	if attempt.StartedAt != nil {
		attempt.DurationMs = now.Sub(*attempt.StartedAt).Milliseconds()
	}
	for _, ev := range out.Evidence {
		if attempt.HTTPStatus == 0 && ev.Result.HTTPStatus != 0 {
			attempt.HTTPStatus = ev.Result.HTTPStatus
		}
		if attempt.RequestID == "" && ev.Result.RequestID != "" {
			attempt.RequestID = ev.Result.RequestID
		}
		attempt.Retries += ev.Retries
	}
	r.db.Save(&attempt)
	if attempt.AttemptNo > 0 {
		r.applyAttempt(attempt)
	}
}

func (r *Runner) applyAttempt(attempt CaseAttempt) {
	var row CaseResult
	if err := r.db.Where("run_id = ? AND case_id = ?", attempt.RunID, attempt.CaseID).First(&row).Error; err != nil {
		return
	}
	row.Status = attempt.Status
	row.HTTPStatus = attempt.HTTPStatus
	row.DurationMs = attempt.DurationMs
	row.Retries = attempt.Retries
	row.RequestID = attempt.RequestID
	row.Summary = attempt.Summary
	row.Metrics = attempt.Metrics
	row.Defects = attempt.Defects
	row.Evidence = attempt.Evidence
	row.EvidenceSHA = attempt.EvidenceSHA
	row.StartedAt = attempt.StartedAt
	row.FinishedAt = attempt.FinishedAt
	r.db.Save(&row)
	r.recomputeRun(attempt.RunID)
}

func (r *Runner) recomputeRun(runID string) {
	var rows []CaseResult
	if err := r.db.Where("run_id = ?", runID).Find(&rows).Error; err != nil {
		return
	}
	counts := map[string]int{}
	for _, row := range rows {
		if row.Status == suite.StatusFailed {
			counts[normalizeLevel(row.Level)]++
		}
	}
	verdict := verdictText(counts)
	status := runSucceeded
	if verdict != "0 项未通过" {
		status = runFailed
	}
	now := time.Now()
	r.db.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
		"status": status, "verdict": verdict, "finished_at": &now, "stop_reason": "",
	})
}

// MarkInterruptedAttempts 在服务重启时清理未完成的复测记录。
func MarkInterruptedAttempts(db *gorm.DB) error {
	now := time.Now()
	return db.Model(&CaseAttempt{}).Where("status IN ?", []string{"queued", "running"}).
		Updates(map[string]any{"status": suite.StatusFailed, "summary": "服务重启，复测中断", "finished_at": &now}).Error
}

// CaseAttempts 返回某任务所有细则的历史尝试摘要，供导出选择器使用。
func (m *Module) runCaseAttempts(runID string) ([]map[string]any, error) {
	var rows []CaseResult
	if err := m.db.Where("run_id = ?", runID).Order("seq ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var attempts []CaseAttempt
	if err := m.db.Where("run_id = ?", runID).Order("case_id ASC, attempt_no ASC").Find(&attempts).Error; err != nil {
		return nil, err
	}
	byCase := map[string][]CaseAttempt{}
	for _, a := range attempts {
		a.Evidence = ""
		byCase[a.CaseID] = append(byCase[a.CaseID], a)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		current := row
		current.Evidence = ""
		attemptsForCase := byCase[row.CaseID]
		if attemptsForCase == nil {
			attemptsForCase = []CaseAttempt{}
		}
		out = append(out, map[string]any{
			"case_id": row.CaseID, "name": row.Name, "level": row.Level, "group": row.Group,
			"current": current, "attempts": attemptsForCase,
		})
	}
	return out, nil
}

// attemptsToCase 把历史 attempt 还原为报告行；未选择时使用当前结果。
func attemptsToCase(row CaseResult, attempt *CaseAttempt) CaseResult {
	if attempt == nil {
		return row
	}
	row.Status = attempt.Status
	row.HTTPStatus = attempt.HTTPStatus
	row.DurationMs = attempt.DurationMs
	row.Retries = attempt.Retries
	row.RequestID = attempt.RequestID
	row.Summary = attempt.Summary
	row.Metrics = attempt.Metrics
	row.Defects = attempt.Defects
	row.Evidence = attempt.Evidence
	row.EvidenceSHA = attempt.EvidenceSHA
	row.StartedAt = attempt.StartedAt
	row.FinishedAt = attempt.FinishedAt
	return row
}

// customExportRows 按 case -> attempt id 映射选择历史结果；缺省取当前。
func (m *Module) customExportRows(runID string, sources map[string]string) ([]CaseResult, error) {
	var rows []CaseResult
	if err := m.db.Where("run_id = ?", runID).Order("seq ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return rows, nil
	}
	var attempts []CaseAttempt
	if err := m.db.Where("run_id = ?", runID).Find(&attempts).Error; err != nil {
		return nil, err
	}
	byID := map[string]CaseAttempt{}
	for _, a := range attempts {
		byID[fmt.Sprint(a.ID)] = a
	}
	for i := range rows {
		if source, ok := sources[rows[i].CaseID]; ok && source != "" && source != "current" {
			if a, ok := byID[source]; ok {
				rows[i] = attemptsToCase(rows[i], &a)
			}
		}
	}
	return rows, nil
}

// custom export helper used by module handler.
func (m *Module) customExportSources(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
