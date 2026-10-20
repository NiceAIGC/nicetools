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
	// Tier 是目标档位：F=仅 F，M=F+M，S=F+M+S（三档顺序执行，失败仍继续）。
	Tier string `json:"tier"`
	// Channel 是渠道标识（如「张三的 K3 渠道」），进入报告环境；导出时可覆盖。
	Channel string `json:"channel"`
	// Declarations 是能力声明 JSON：{"video":false,...}；未声明支持的能力对应用例豁免。
	Declarations string `json:"declarations"`
	// CaseOverrides 是用例级自定义 JSON：{"M-30":"skip"|"exempt"}；
	// skip=不测试（跳过），exempt=手动豁免（不计入失败）。
	CaseOverrides string `json:"case_overrides"`
	// MediaToken 是测试媒体（视频/图片）公开拉取的一次性令牌，不进 API 响应。
	MediaToken string `json:"-"`
	// Status: running / pausing / paused / succeeded / failed / aborted
	Status  string `gorm:"index" json:"status"`
	Verdict string `json:"verdict"` // 通过 / 不通过 / 无效（闸门不过）/ ""
	// TierResults 是每档判定 JSON：{"F":"passed","M":"failed"}。
	TierResults string     `json:"tier_results"`
	StopReason  string     `json:"stop_reason,omitempty"`
	TotalCases  int        `json:"total_cases"`
	DoneCases   int        `json:"done_cases"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
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

// CaseCfg 是管理员级用例永久配置（规范 §6 第三层）：
// mode=skip/exempt 给所有新任务的默认值；level 非空时覆盖用例级别（P0/P1/P2/P3）。
type CaseCfg struct {
	Suite  string `gorm:"primaryKey" json:"suite"`
	CaseID string `gorm:"primaryKey" json:"case_id"`
	Mode   string `json:"mode"`  // "" / skip / exempt
	Level  string `json:"level"` // "" / P0 / P1 / P2 / P3
}

func (CaseCfg) TableName() string { return "tool_llmadmit_casecfg" }
