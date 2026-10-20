package suite

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Usage 是 OpenAI 兼容的 token 计量结构。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// completion_tokens_details.reasoning_tokens
	ReasoningTokens int `json:"reasoning_tokens"`
	// prompt_tokens_details.cached_tokens
	CachedTokens int `json:"cached_tokens"`
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	var raw struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	u.PromptTokens = raw.PromptTokens
	u.CompletionTokens = raw.CompletionTokens
	u.TotalTokens = raw.TotalTokens
	u.CachedTokens = raw.PromptDetails.CachedTokens
	u.ReasoningTokens = raw.CompletionDetails.ReasoningTokens
	return nil
}

// CallResult 是一次请求的完整留档：传输层、三口径计时、内容与原文。
type CallResult struct {
	HTTPStatus int    `json:"http_status"`
	RequestID  string `json:"request_id,omitempty"`
	// TTFT：请求发出 → 首个 SSE chunk；非流式时等于 E2E。
	TTFTMs int64 `json:"ttft_ms"`
	// TTFT-业务：请求发出 → 首个非空 delta.content（§2）。
	TTFTContentMs int64   `json:"ttft_content_ms"`
	E2EMs         int64   `json:"e2e_ms"`
	Chunks        int     `json:"chunks"`
	ITLMsa        []int64 `json:"itl_ms,omitempty"`
	Content       string  `json:"content"`
	Reasoning     string  `json:"reasoning,omitempty"`
	FinishReason  string  `json:"finish_reason"`
	Usage         *Usage  `json:"usage,omitempty"`
	// Raw 是完整响应原文（SSE 或 JSON），超过 RawCap 截断并记录 RawSHA 全文哈希。
	Raw    string `json:"raw"`
	RawSHA string `json:"raw_sha"`
	// ErrKind：transport(连接/超时/断流) / ""(HTTP 层正常返回)。
	ErrKind string `json:"err_kind,omitempty"`
	ErrMsg  string `json:"err_msg,omitempty"`
}

const RawCap = 256 * 1024

// Client 是 OpenAI 兼容网关的最小客户端。
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: TimeoutConnect}).DialContext,
			ResponseHeaderTimeout: TimeoutNormal,
			MaxIdleConns:          64,
			IdleConnTimeout:       90 * time.Second,
		},
	}}
}

// Get 执行简单 GET（如 /v1/models），返回状态码与正文（截断）。
// 带整体超时：连接 30s 之外，正文读取同样受限（历史上正文挂死导致用例卡死）。
func (c *Client) Get(ctx context.Context, t Target, path string) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, TimeoutNormal)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(t.BaseURL, "/")+path, nil)
	if err != nil {
		return 0, "", err
	}
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, string(body), err
}

// Chat 调用 /chat/completions。body 中 stream 决定走 SSE 还是一次性 JSON；
// timeout 覆盖默认 180s（大输入/大输出场景）。
func (c *Client) Chat(ctx context.Context, t Target, body map[string]any, timeout time.Duration) CallResult {
	if _, ok := body["model"]; !ok {
		body["model"] = t.Model
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	stream, _ := body["stream"].(bool)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, strings.TrimSuffix(t.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	start := time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		return CallResult{ErrKind: "transport", ErrMsg: err.Error(), E2EMs: ms(start)}
	}
	defer res.Body.Close()
	r := CallResult{HTTPStatus: res.StatusCode, RequestID: res.Header.Get("X-Request-Id")}
	if r.RequestID == "" {
		r.RequestID = res.Header.Get("Request-Id")
	}
	if stream && res.StatusCode == 200 {
		c.readSSE(reqCtx, res.Body, &r, start)
	} else {
		c.readJSON(res.Body, &r, start)
	}
	return r
}

func ms(start time.Time) int64 { return time.Since(start).Milliseconds() }

func finalizeRaw(r *CallResult, raw []byte) {
	sum := sha256.Sum256(raw)
	r.RawSHA = hex.EncodeToString(sum[:])
	if len(raw) > RawCap {
		r.Raw = string(raw[:RawCap]) + "\n…[truncated]"
	} else {
		r.Raw = string(raw)
	}
}

func (c *Client) readJSON(body io.Reader, r *CallResult, start time.Time) {
	raw, err := io.ReadAll(io.LimitReader(body, 4<<20))
	r.E2EMs, r.TTFTMs, r.TTFTContentMs = ms(start), ms(start), ms(start)
	finalizeRaw(r, raw)
	if err != nil {
		r.ErrKind, r.ErrMsg = "transport", err.Error()
		return
	}
	if r.HTTPStatus != 200 {
		return
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   json.RawMessage `json:"content"`
				Reasoning string          `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		r.ErrKind, r.ErrMsg = "client", "响应 JSON 解析失败: "+err.Error()
		return
	}
	if len(parsed.Choices) > 0 {
		r.Content = contentText(parsed.Choices[0].Message.Content)
		r.Reasoning = parsed.Choices[0].Message.Reasoning
		r.FinishReason = parsed.Choices[0].FinishReason
	}
	r.Usage = parsed.Usage
}

func (c *Client) readSSE(ctx context.Context, body io.Reader, r *CallResult, start time.Time) {
	var raw bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(body, &raw))
	var content, reasoning strings.Builder
	var usage *Usage
	var lastChunk time.Time
	firstContent := false
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if data == "[DONE]" {
					break
				}
				now := time.Now()
				if r.Chunks == 0 {
					r.TTFTMs = now.Sub(start).Milliseconds()
				} else if len(r.ITLMsa) < 4096 {
					r.ITLMsa = append(r.ITLMsa, now.Sub(lastChunk).Milliseconds())
				}
				lastChunk = now
				r.Chunks++
				var chunk struct {
					Choices []struct {
						Delta struct {
							Content   json.RawMessage `json:"content"`
							Reasoning string          `json:"reasoning_content"`
						} `json:"delta"`
						FinishReason *string `json:"finish_reason"`
					} `json:"choices"`
					Usage *Usage `json:"usage"`
				}
				if json.Unmarshal([]byte(data), &chunk) == nil {
					if len(chunk.Choices) > 0 {
						text := contentText(chunk.Choices[0].Delta.Content)
						if text != "" {
							if !firstContent {
								r.TTFTContentMs = now.Sub(start).Milliseconds()
								firstContent = true
							}
							content.WriteString(text)
						}
						reasoning.WriteString(chunk.Choices[0].Delta.Reasoning)
						if chunk.Choices[0].FinishReason != nil {
							r.FinishReason = *chunk.Choices[0].FinishReason
						}
					}
					if chunk.Usage != nil {
						usage = chunk.Usage
					}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				r.ErrKind, r.ErrMsg = "transport", "断流: "+err.Error()
			}
			break
		}
		if ctx.Err() != nil {
			r.ErrKind, r.ErrMsg = "transport", ctx.Err().Error()
			break
		}
	}
	r.E2EMs = ms(start)
	r.Content, r.Reasoning, r.Usage = content.String(), reasoning.String(), usage
	finalizeRaw(r, raw.Bytes())
}

// content 既可能是字符串也可能是分段数组（多模态响应）。
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return ""
}

// Models 校验 /v1/models 中目标模型在列（前置闸门 G1）。
func (c *Client) Models(ctx context.Context, t Target) (int, []string, error) {
	status, body, err := c.Get(ctx, t, "/models")
	if status != 200 || err != nil {
		return status, nil, err
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return status, nil, fmt.Errorf("models 响应解析失败")
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		ids = append(ids, m.ID)
	}
	return status, ids, nil
}
