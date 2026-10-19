package llmadmit

import "time"

// Run 是一次准入测试任务。APIKey 只进不出（json:"-"），导出报告一律脱敏。
type Run struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CreatedBy string `gorm:"index" json:"created_by"`
	Suite     string `json:"suite"`
	SuiteName string `json:"suite_name"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"-"`
	Model     string `json:"model"`
	// Tier 是目标档位：F=仅 F，M=F+M，S=F+M+S（三档顺序执行，失败即终止）。
	Tier string `json:"tier"`
	// Declarations 是能力声明 JSON：{"video":false,...}；未声明支持的能力对应用例豁免。
	Declarations string `json:"declarations"`
	// Status: running / pausing / paused / succeeded / failed / aborted
	Status     string     `gorm:"index" json:"status"`
	Verdict    string     `json:"verdict"` // 通过 / 不通过 / 无效（闸门不过）/ ""
	StopReason string     `json:"stop_reason,omitempty"`
	TotalCases int        `json:"total_cases"`
	DoneCases  int        `json:"done_cases"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func (Run) TableName() string { return "tool_llmadmit_runs" }

// CaseResult 是单用例执行留档；Evidence 保留请求/响应原文（截断 256KB）+ SHA256。
type CaseResult struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	RunID       string     `gorm:"index:idx_llmadmit_case,priority:1" json:"run_id"`
	Seq         int        `gorm:"index:idx_llmadmit_case,priority:2" json:"seq"`
	CaseID      string     `json:"case_id"`
	Tier        string     `json:"tier"`
	Name        string     `json:"name"`
	Level       string     `json:"level"`
	Status      string     `gorm:"index" json:"status"`
	HTTPStatus  int        `json:"http_status"`
	DurationMs  int64      `json:"duration_ms"`
	Retries     int        `json:"retries"`
	RequestID   string     `json:"request_id,omitempty"`
	Summary     string     `json:"summary"`
	Metrics     string     `json:"metrics"`  // JSON object
	Defects     string     `json:"defects"`  // JSON array
	Evidence    string     `json:"evidence"` // 请求体 + 响应原文（截断）
	EvidenceSHA string     `json:"evidence_sha"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func (CaseResult) TableName() string { return "tool_llmadmit_cases" }
