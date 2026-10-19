package llmadmit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"nicetools/backend/internal/config"
	"nicetools/backend/internal/service"
	"nicetools/backend/internal/store"
	"nicetools/backend/internal/tools/llmadmit/k3"
	"nicetools/backend/internal/tools/llmadmit/suite"
)

// fakeGateway 实现 K3 官方契约的最小 OpenAI 兼容网关：
// 采样参数固定值校验、始终思考、usage 三字段、流式 [DONE] 收尾。
func fakeGateway(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"kimi-k3","context_length":1048576}]}`))
			return
		}
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"message":"invalid key"}}`))
			return
		}
		var body struct {
			Model         string           `json:"model"`
			Messages      []map[string]any `json:"messages"`
			MaxTokens     int              `json:"max_tokens"`
			MaxCompletion int              `json:"max_completion_tokens"`
			Temperature   *float64         `json:"temperature"`
			TopP          *float64         `json:"top_p"`
			N             *int             `json:"n"`
			Stream        bool             `json:"stream"`
			StreamOptions map[string]any   `json:"stream_options"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		if body.Temperature != nil && *body.Temperature != 1.0 {
			w.WriteHeader(400)
			return
		}
		if body.TopP != nil && *body.TopP != 0.95 {
			w.WriteHeader(400)
			return
		}
		if body.N != nil && *body.N != 1 {
			w.WriteHeader(400)
			return
		}
		prompt, isImage := extractPrompt(body.Messages)
		content := "我是 Kimi，由 Moonshot AI 研发。"
		if isImage {
			content = "左上角是红色，右上角是绿色，左下角是蓝色，右下角是黄色。"
		}
		if strings.Contains(prompt, "数到") || strings.Contains(prompt, "一句话") {
			content = "1、2、3、4、5。" + content
		}
		usage := map[string]any{
			"prompt_tokens":             max(utf8.RuneCountInString(prompt), 1),
			"completion_tokens":         24,
			"total_tokens":              max(utf8.RuneCountInString(prompt), 1) + 24,
			"prompt_tokens_details":     map[string]any{"cached_tokens": 0},
			"completion_tokens_details": map[string]any{"reasoning_tokens": 8},
		}
		finish := "stop"
		if (body.MaxTokens > 0 && body.MaxTokens <= 32) || (body.MaxCompletion > 0 && body.MaxCompletion <= 32) {
			finish = "length"
		}
		w.Header().Set("X-Request-Id", "req-fake-1")
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			chunks := []string{"我是", " Kimi", "。"}
			for i, piece := range chunks {
				delta := map[string]any{"content": piece}
				if i == 0 {
					delta["reasoning_content"] = "思考"
				}
				chunk := map[string]any{"choices": []any{map[string]any{"delta": delta}}}
				if i == len(chunks)-1 {
					chunk["choices"] = []any{map[string]any{"delta": delta, "finish_reason": finish}}
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			if body.StreamOptions != nil {
				usageChunk, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": usage})
				fmt.Fprintf(w, "data: %s\n\n", usageChunk)
			}
			w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		response := map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion", "model": "kimi-k3",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": content, "reasoning_content": "思考过程"},
				"finish_reason": finish,
			}},
			"usage": usage,
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// extractPrompt 汇总消息文本；返回是否含多模态图片。
func extractPrompt(messages []map[string]any) (string, bool) {
	var sb strings.Builder
	image := false
	for _, m := range messages {
		switch content := m["content"].(type) {
		case string:
			sb.WriteString(content)
		case []any:
			for _, part := range content {
				if p, ok := part.(map[string]any); ok {
					if p["type"] == "image_url" {
						image = true
					}
					if text, ok := p["text"].(string); ok {
						sb.WriteString(text)
					}
				}
			}
		}
	}
	return sb.String(), image
}

func setupRunner(t *testing.T) (*Runner, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{DBPath: filepath.Join(root, "test.db")}
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Module{}).Migrate(st.DB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewRunner(st.DB, service.New(st), Suites()), st
}

func waitStatus(t *testing.T, r *Runner, runID string, deadline time.Duration, want ...string) Run {
	t.Helper()
	until := time.Now().Add(deadline)
	for time.Now().Before(until) {
		run, err := r.loadRun(runID)
		if err == nil {
			for _, s := range want {
				if run.Status == s {
					return run
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	run, _ := r.loadRun(runID)
	t.Fatalf("等待状态 %v 超时，当前 %s", want, run.Status)
	return Run{}
}

// F 档全量：假网关下应整体通过，用例全部有结论。
func TestRunnerFTierPasses(t *testing.T) {
	server := fakeGateway(0)
	defer server.Close()
	r, st := setupRunner(t)
	run, err := r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	final := waitStatus(t, r, run.ID, 60*time.Second, runSucceeded, runFailed, runAborted)
	if final.Verdict != "通过" {
		var rows []CaseResult
		st.DB.Where("run_id = ?", run.ID).Order("seq ASC").Find(&rows)
		for _, row := range rows {
			if row.Status != suite.StatusPassed && row.Status != suite.StatusSkipped && row.Status != suite.StatusExempt {
				t.Logf("%s-%s %s: %s (%s)", row.Tier, row.CaseID, row.Name, row.Status, row.Summary)
			}
		}
		t.Fatalf("verdict=%s want 通过", final.Verdict)
	}
	if final.DoneCases != final.TotalCases {
		t.Fatalf("done=%d total=%d", final.DoneCases, final.TotalCases)
	}
	// 每个用例都落了证据与哈希（§7.3 证据保全）。
	var empty int64
	st.DB.Model(&CaseResult{}).Where("run_id = ? AND status = ? AND evidence_sha = ''", run.ID, suite.StatusPassed).Count(&empty)
	if empty > 0 {
		t.Fatalf("%d 个通过用例缺证据哈希", empty)
	}
}

// 单飞：活跃任务存在时第二个任务 409。
func TestRunnerSingleFlight(t *testing.T) {
	server := fakeGateway(150 * time.Millisecond)
	defer server.Close()
	r, _ := setupRunner(t)
	run, err := r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	problem, ok := err.(*service.Error)
	if !ok || problem.Status != 409 {
		t.Fatalf("second start err=%v want 409", err)
	}
	_ = r.Abort(run.ID, "tester")
}

// 暂停/继续：暂停在用例边界生效，恢复后跑完。
func TestRunnerPauseResume(t *testing.T) {
	server := fakeGateway(120 * time.Millisecond)
	defer server.Close()
	r, _ := setupRunner(t)
	run, err := r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := r.Pause(run.ID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, r, run.ID, 30*time.Second, runPaused)
	if err := r.Resume(run.ID); err != nil {
		t.Fatal(err)
	}
	final := waitStatus(t, r, run.ID, 60*time.Second, runSucceeded, runFailed)
	if final.Verdict != "通过" {
		t.Fatalf("verdict=%s want 通过", final.Verdict)
	}
	if final.DoneCases != final.TotalCases {
		t.Fatalf("恢复后进度不完整: %d/%d", final.DoneCases, final.TotalCases)
	}
}

// 档位失败不终止：P0 失败后当档剩余用例继续，任务跑完，裁决不通过。
func TestRunnerTierFailureContinues(t *testing.T) {
	// 假网关身份自报错误 → F-02 P0 失败。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"kimi-k3"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","model":"kimi-k3","choices":[{"message":{"role":"assistant","content":"我是 ChatGPT","reasoning_content":"想"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":24,"total_tokens":33,"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":8}}}`))
	}))
	defer server.Close()
	r, st := setupRunner(t)
	run, err := r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	final := waitStatus(t, r, run.ID, 60*time.Second, runSucceeded, runFailed)
	if final.Verdict != "不通过" {
		t.Fatalf("verdict=%s want 不通过", final.Verdict)
	}
	// 失败不提前终止：所有用例都有终态。
	if final.DoneCases != final.TotalCases {
		t.Fatalf("done=%d total=%d（失败不应终止剩余用例）", final.DoneCases, final.TotalCases)
	}
	var pending int64
	st.DB.Model(&CaseResult{}).Where("run_id = ? AND status IN ?", run.ID, []string{suite.StatusPending, suite.StatusRunning}).Count(&pending)
	if pending > 0 {
		t.Fatalf("%d 个用例未执行", pending)
	}
	// 每档判定落库。
	if !strings.Contains(final.TierResults, `"F":"failed"`) {
		t.Fatalf("tier_results=%s", final.TierResults)
	}
}

// 闸门不过 → 测试无效，不产出 P0 结论。
func TestRunnerGateFailureInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
	}))
	defer server.Close()
	r, _ := setupRunner(t)
	run, err := r.Start("tester", k3.Suite(), suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"}, "F", "", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	final := waitStatus(t, r, run.ID, 15*time.Second, runFailed)
	if final.Verdict != "无效" {
		t.Fatalf("verdict=%s want 无效", final.Verdict)
	}
}

// Recover：服务重启把中断任务标记 aborted。
func TestRunnerRecover(t *testing.T) {
	r, st := setupRunner(t)
	run := Run{ID: "x1", CreatedBy: "t", Suite: "k3-v1", BaseURL: "http://x", Model: "kimi-k3", Tier: "F", Status: runRunning, CreatedAt: time.Now()}
	if err := st.DB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.Recover(); err != nil {
		t.Fatal(err)
	}
	after, _ := r.loadRun("x1")
	if after.Status != runAborted {
		t.Fatalf("status=%s want aborted", after.Status)
	}
}
