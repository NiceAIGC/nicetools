package llmadmit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"nicetools/backend/internal/module"
	"nicetools/backend/internal/service"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

// caseEvidenceItem 是单次请求的可复制证据；请求与响应均已在服务端脱敏。
type caseEvidenceItem struct {
	Label           string            `json:"label"`
	Request         string            `json:"request"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	Response        string            `json:"response"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	HTTPStatus      int               `json:"http_status"`
	FinishReason    string            `json:"finish_reason,omitempty"`
	DurationMs      int64             `json:"duration_ms,omitempty"`
	Retries         int               `json:"retries,omitempty"`
}

type caseEvidenceResult struct {
	CaseID      string             `json:"case_id"`
	Status      string             `json:"status"`
	Summary     string             `json:"summary"`
	Doc         suite.CaseDoc      `json:"doc"`
	Defects     []suite.Defect     `json:"defects"`
	Explanation string             `json:"explanation"`
	Evidence    []caseEvidenceItem `json:"evidence"`
}

func redactHeaders(headers map[string]string, baseURL string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "x-api-key" || lower == "api-key" || lower == "set-cookie" {
			out[key] = "***"
			continue
		}
		out[key] = redact(value, baseURL)
	}
	return out
}

// caseEvidence 返回单个用例的脱敏请求/响应，按需加载，避免任务详情一次传输全部原文。
func (m *Module) caseEvidence(c *gin.Context) {
	var run Run
	if err := m.db.First(&run, "id = ?", c.Param("rid")).Error; err != nil {
		module.Fail(c, service.DBError(err))
		return
	}
	var row CaseResult
	if err := m.db.Where("run_id = ? AND case_id = ?", run.ID, c.Param("cid")).First(&row).Error; err != nil {
		module.Fail(c, service.Problem(404, "用例不存在"))
		return
	}
	doc := suite.CaseDoc{}
	if m.runner != nil {
		if s, ok := m.runner.suites[run.Suite]; ok {
			for _, cs := range s.Cases {
				if cs.ID == row.CaseID {
					doc = cs.Doc
					break
				}
			}
		}
	}
	var raw []suite.Evidence
	_ = json.Unmarshal([]byte(row.Evidence), &raw)
	items := make([]caseEvidenceItem, 0, len(raw))
	for _, item := range raw {
		items = append(items, caseEvidenceItem{
			Label:           item.Label,
			Request:         redact(item.Request, run.BaseURL),
			RequestHeaders:  redactHeaders(item.Result.RequestHeaders, run.BaseURL),
			Response:        redact(item.Result.Raw, run.BaseURL),
			ResponseHeaders: redactHeaders(item.Result.ResponseHeaders, run.BaseURL),
			HTTPStatus:      item.Result.HTTPStatus,
			FinishReason:    item.Result.FinishReason,
			DurationMs:      item.Result.E2EMs,
			Retries:         item.Retries,
		})
	}
	var defects []suite.Defect
	_ = json.Unmarshal([]byte(row.Defects), &defects)
	if defects == nil {
		defects = []suite.Defect{}
	}
	c.JSON(200, caseEvidenceResult{
		CaseID: row.CaseID, Status: row.Status, Summary: row.Summary,
		Doc: doc, Defects: defects, Explanation: explainCase(row, doc, defects), Evidence: items,
	})
}

func cleanSentence(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "。；; ")
}

// explainCase 生成给人看的结论说明，不做模型调用，避免解释本身引入额外不确定性。
func explainCase(row CaseResult, doc suite.CaseDoc, defects []suite.Defect) string {
	switch row.Status {
	case suite.StatusSkipped:
		return "该用例本次未执行（跳过），所以没有请求/返回证据。"
	case suite.StatusExempt:
		return "该用例因能力未声明或手动跳过，本次未执行；不是模型失败。"
	case suite.StatusPassed:
		msg := "该用例通过。"
		if row.Summary != "" {
			msg += "实测：" + row.Summary + "。"
		}
		if doc.Expect != "" {
			msg += "判据：" + cleanSentence(doc.Expect) + "。"
		}
		return msg
	}

	var b strings.Builder
	b.WriteString("该用例未通过。")
	if row.Summary != "" {
		b.WriteString("实测：" + row.Summary + "。")
	}
	if len(defects) > 0 {
		b.WriteString("关键原因：")
		for i, d := range defects {
			if i > 0 {
				b.WriteString("；")
			}
			b.WriteString(fmt.Sprintf("期望 %s，实际 %s", d.Expect, d.Actual))
		}
		b.WriteString("。")
	}
	if doc.Expect != "" {
		b.WriteString("原始判据：" + cleanSentence(doc.Expect) + "。")
	}
	return b.String()
}

// exportFailedEvidence 导出未通过用例的脱敏请求/返回，供直接发给供应商。
// format=md（默认）或 txt；只包含 failed，跳过 passed/skipped/exempt。
func (m *Module) exportFailedEvidence(c *gin.Context) {
	var run Run
	if err := m.db.First(&run, "id = ?", c.Param("rid")).Error; err != nil {
		module.Fail(c, service.DBError(err))
		return
	}
	var rows []CaseResult
	if err := m.db.Where("run_id = ? AND status = ?", run.ID, suite.StatusFailed).Order("seq ASC").Find(&rows).Error; err != nil {
		module.Fail(c, err)
		return
	}
	if len(rows) == 0 {
		module.Fail(c, service.Problem(404, "该任务没有未通过用例"))
		return
	}
	format := strings.ToLower(c.DefaultQuery("format", "md"))
	if format != "md" && format != "txt" {
		module.Fail(c, service.Problem(400, "format 仅支持 md/txt"))
		return
	}
	docs := map[string]suite.CaseDoc{}
	if m.runner != nil {
		if s, ok := m.runner.suites[run.Suite]; ok {
			for _, cs := range s.Cases {
				docs[cs.ID] = cs.Doc
			}
		}
	}
	var b strings.Builder
	if format == "md" {
		fmt.Fprintf(&b, "# 未通过用例证据\n\n")
		fmt.Fprintf(&b, "- 模型：`%s`\n- 任务：`%s`\n- 网关：`%s`\n- 未通过：%d 项\n\n", run.Model, run.ID, maskURL(run.BaseURL), len(rows))
	} else {
		fmt.Fprintf(&b, "未通过用例证据\n模型：%s\n任务：%s\n网关：%s\n未通过：%d 项\n\n", run.Model, run.ID, maskURL(run.BaseURL), len(rows))
	}
	for _, row := range rows {
		doc := docs[row.CaseID]
		var defects []suite.Defect
		_ = json.Unmarshal([]byte(row.Defects), &defects)
		if format == "md" {
			fmt.Fprintf(&b, "## %s · %s\n\n", row.CaseID, row.Name)
			fmt.Fprintf(&b, "- 级别：%s\n- 结论：%s\n- 实测：%s\n", row.Level, row.Status, row.Summary)
			if doc.Expect != "" {
				fmt.Fprintf(&b, "- 判据：%s\n", cleanSentence(doc.Expect))
			}
			for _, d := range defects {
				fmt.Fprintf(&b, "- 缺陷：期望 %s；实际 %s\n", d.Expect, d.Actual)
			}
			b.WriteString("\n")
		} else {
			fmt.Fprintf(&b, "===== %s · %s =====\n级别：%s\n结论：%s\n实测：%s\n", row.CaseID, row.Name, row.Level, row.Status, row.Summary)
			if doc.Expect != "" {
				fmt.Fprintf(&b, "判据：%s\n", cleanSentence(doc.Expect))
			}
			for _, d := range defects {
				fmt.Fprintf(&b, "缺陷：期望 %s；实际 %s\n", d.Expect, d.Actual)
			}
			b.WriteString("\n")
		}
		var evidence []suite.Evidence
		_ = json.Unmarshal([]byte(row.Evidence), &evidence)
		for i, item := range evidence {
			label := item.Label
			if label == "" {
				label = fmt.Sprintf("请求 %d", i+1)
			}
			request := redact(item.Request, run.BaseURL)
			response := redact(item.Result.Raw, run.BaseURL)
			if format == "md" {
				fmt.Fprintf(&b, "### %s\n\n**HTTP %d**\n\n```json\n%s\n```\n\n", label, item.Result.HTTPStatus, request)
				fmt.Fprintf(&b, "```text\n%s\n```\n\n", response)
			} else {
				fmt.Fprintf(&b, "--- %s · HTTP %d ---\n请求：\n%s\n\n返回：\n%s\n\n", label, item.Result.HTTPStatus, request, response)
			}
		}
	}
	ext := "md"
	contentType := "text/markdown; charset=utf-8"
	if format == "txt" {
		ext = "txt"
		contentType = "text/plain; charset=utf-8"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="llmadmit-failed-%s-%s.%s"`, exportName(primaryDomain(run.BaseURL)), run.ID, ext))
	c.Data(200, contentType, []byte(b.String()))
}
