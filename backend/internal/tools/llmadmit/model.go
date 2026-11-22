package llmadmit

import "time"

// RunProgress 是运行中任务的实时进度（仅内存展示，不落库）。
type RunProgress struct {
	CaseID    string    `json:"case_id"`
	CaseName  string    `json:"case_name"`
	Step      string    `json:"step,omitempty"`
	Done      int       `json:"done"`
	Total     int       `json:"total"`
	Requests  int       `json:"requests"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Run 是一次准入测试任务。APIKey 只进不出（json:"-"），导出报告一律脱敏。
type Run struct {
	ID            string `gorm:"primaryKey" json:"id"`
	CreatedBy     string `gorm:"index" json:"created_by"`
	Suite         string `json:"suite"`
	SuiteName     string `json:"suite_name"`
	BaseURL       string `json:"base_url"`
	PrimaryDomain string `gorm:"index" json:"primary_domain"`
	APIKey        string `json:"-"`
	Model         string `json:"model"`
	// Preset 是本次测试范围：""（全测）/format/cache/latency。
	Preset string `json:"preset"`
	// LegacyTier 仅读取旧任务的档位字段，保证升级前历史报告仍可识别范围。
	LegacyTier string `gorm:"column:tier" json:"-"`
	// Channel 是渠道标识（如「张三的 K3 渠道」），进入报告环境；导出时可覆盖。
	Channel string `json:"channel"`
	// Declarations 是能力声明 JSON：{"video":false,...}；未声明支持的能力对应用例豁免。
	Declarations string `json:"declarations"`
	// CaseOverrides 是用例级自定义 JSON：{"M-30":"skip"|"exempt"}；
	// skip=不测试（跳过），exempt=手动豁免（不计入失败）。
	CaseOverrides string `json:"case_overrides"`
	// CaseIDs 是显式选择的用例 ID（逗号分隔）；非空时本次只跑这些用例（闸门始终加入）。
	CaseIDs string `json:"case_ids,omitempty"`
	// MediaToken 是测试媒体（视频/图片）公开拉取的一次性令牌，不进 API 响应。
	MediaToken string `json:"-"`
	// Status: queued / running / pausing / paused / succeeded / failed / aborted
	Status     string     `gorm:"index" json:"status"`
	Verdict    string     `json:"verdict"` // 通过 / 不通过 / 无效（闸门不过）/ ""
	StopReason string     `json:"stop_reason,omitempty"`
	TotalCases int        `json:"total_cases"`
	DoneCases  int        `json:"done_cases"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// 以下字段只用于 API 展示，不落库；由用例结果即时计算。
	Passed   int          `gorm:"-" json:"passed"`
	Failed   int          `gorm:"-" json:"failed"`
	Assessed int          `gorm:"-" json:"assessed"`
	PassRate float64      `gorm:"-" json:"pass_rate"`
	Grade    string       `gorm:"-" json:"grade"`
	Progress *RunProgress `gorm:"-" json:"progress,omitempty"`
}

func (Run) TableName() string { return "tool_llmadmit_runs" }

// CaseResult 是单用例执行留档；Evidence 保留请求/响应原文（截断 256KB）+ SHA256。
type CaseResult struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	RunID  string `gorm:"index:idx_llmadmit_case,priority:1" json:"run_id"`
	Seq    int    `gorm:"index:idx_llmadmit_case,priority:2" json:"seq"`
	CaseID string `json:"case_id"`
	// LegacyTier 保留旧数据的档位列，仅用于历史报告回显。
	LegacyTier  string     `gorm:"column:tier" json:"-"`
	Group       string     `json:"group"` // gate / format / cache / latency
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

// CaseAttempt 是单个细则的历史执行记录：原始结果 + 每次单项复测。
// CaseResult 始终保存当前最新结果；历史选择用于导出。
type CaseAttempt struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	RunID       string     `gorm:"index:idx_llmadmit_attempt,priority:1" json:"run_id"`
	CaseID      string     `gorm:"index:idx_llmadmit_attempt,priority:2" json:"case_id"`
	AttemptNo   int        `json:"attempt_no"` // 0=原始结果，1..=复测
	Source      string     `json:"source"`     // original / recheck
	Status      string     `json:"status"`
	HTTPStatus  int        `json:"http_status"`
	DurationMs  int64      `json:"duration_ms"`
	Retries     int        `json:"retries"`
	RequestID   string     `json:"request_id,omitempty"`
	Summary     string     `json:"summary"`
	Metrics     string     `json:"metrics"`
	Defects     string     `json:"defects"`
	Evidence    string     `json:"evidence"`
	EvidenceSHA string     `json:"evidence_sha"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func (CaseAttempt) TableName() string { return "tool_llmadmit_attempts" }

// Setting 保存工具级运行配置，目前用于最大并行任务数。
type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

func (Setting) TableName() string { return "tool_llmadmit_settings" }

// CaseCfg 是管理员级用例永久配置（规范 §6 第三层）：
// mode=skip/exempt 给所有新任务的默认值；level 非空时覆盖用例级别（P0/P1/P2/P3）。
type CaseCfg struct {
	Suite  string `gorm:"primaryKey" json:"suite"`
	CaseID string `gorm:"primaryKey" json:"case_id"`
	Mode   string `json:"mode"`  // "" / skip / exempt
	Level  string `json:"level"` // "" / P0 / P1 / P2 / P3
}

func (CaseCfg) TableName() string { return "tool_llmadmit_casecfg" }
