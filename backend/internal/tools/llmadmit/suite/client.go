package suite

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Usage 兼容三种官方响应格式的 token/缓存计量：
// Chat Completions、Responses、Anthropic Messages。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	// completion_tokens_details.reasoning_tokens
	ReasoningTokens int `json:"reasoning_tokens"`
	// prompt_tokens_details.cached_tokens / input_tokens_details.cached_tokens / cache_read_input_tokens
	CachedTokens int `json:"cached_tokens"`
	// prompt_tokens_details.cache_write_tokens / input_tokens_details.cache_write_tokens / cache_creation_input_tokens
	CacheWriteTokens int `json:"cache_write_tokens"`
	// Messages: cache_creation.ephemeral_{5m,1h}_input_tokens
	CacheWrite5m int `json:"cache_write_5m_tokens"`
	CacheWrite1h int `json:"cache_write_1h_tokens"`
}

func maxInt(values ...int) int {
	best := 0
	for _, v := range values {
		if v > best {
			best = v
		}
	}
	return best
}

func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	var raw struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		PromptDetails    struct {
			CachedTokens       int `json:"cached_tokens"`
			CacheWriteTokens   int `json:"cache_write_tokens"`
			CreatedCacheTokens int `json:"created_cache_tokens"`
			CacheWriteDetails  struct {
				TTL5m int `json:"ttl_5m_tokens"`
				TTL1h int `json:"ttl_1h_tokens"`
			} `json:"cache_write_tokens_details"`
		} `json:"prompt_tokens_details"`
		InputDetails struct {
			CachedTokens       int `json:"cached_tokens"`
			CacheWriteTokens   int `json:"cache_write_tokens"`
			CreatedCacheTokens int `json:"created_cache_tokens"`
			CacheWriteDetails  struct {
				TTL5m int `json:"ttl_5m_tokens"`
				TTL1h int `json:"ttl_1h_tokens"`
			} `json:"cache_write_tokens_details"`
		} `json:"input_tokens_details"`
		CompletionDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		CachedTokens       int `json:"cached_tokens"`
		CacheReadTokens    int `json:"cache_read_tokens"`
		CreatedCacheTokens int `json:"created_cache_tokens"`
		CacheWriteTokens   int `json:"cache_write_tokens"`
		CacheWrite1hTokens int `json:"cache_write_1h_tokens"`
		CacheWriteDetails  struct {
			TTL5m int `json:"ttl_5m_tokens"`
			TTL1h int `json:"ttl_1h_tokens"`
		} `json:"cache_write_tokens_details"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheCreation            struct {
			Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
			Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
		} `json:"cache_creation"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	u.PromptTokens = raw.PromptTokens
	u.CompletionTokens = raw.CompletionTokens
	u.TotalTokens = raw.TotalTokens
	u.InputTokens = raw.InputTokens
	u.OutputTokens = raw.OutputTokens
	u.ReasoningTokens = raw.CompletionDetails.ReasoningTokens
	switch {
	case raw.CacheReadInputTokens > 0 || raw.CacheCreationInputTokens > 0:
		// Messages: input_tokens 不含缓存读写，总量需相加。
		u.PromptTokens = raw.InputTokens + raw.CacheReadInputTokens + raw.CacheCreationInputTokens
		u.CachedTokens = raw.CacheReadInputTokens
		u.CacheWriteTokens = raw.CacheCreationInputTokens
		u.CacheWrite5m = raw.CacheCreation.Ephemeral5m
		u.CacheWrite1h = raw.CacheCreation.Ephemeral1h
	case raw.PromptTokens > 0:
		u.CachedTokens = firstPositive(raw.PromptDetails.CachedTokens, raw.CachedTokens, raw.CacheReadTokens)
		u.CacheWriteTokens = maxInt(raw.PromptDetails.CacheWriteTokens, raw.PromptDetails.CreatedCacheTokens, raw.CacheWriteTokens, raw.CreatedCacheTokens)
		u.CacheWrite5m = firstPositive(raw.PromptDetails.CacheWriteDetails.TTL5m, raw.CacheWriteDetails.TTL5m)
		u.CacheWrite1h = firstPositive(raw.PromptDetails.CacheWriteDetails.TTL1h, raw.CacheWriteDetails.TTL1h, raw.CacheWrite1hTokens)
	default:
		// Responses: input_tokens 是总输入，明细在 input_tokens_details。
		u.PromptTokens = raw.InputTokens
		u.CachedTokens = firstPositive(raw.InputDetails.CachedTokens, raw.CachedTokens, raw.CacheReadTokens)
		u.CacheWriteTokens = maxInt(raw.InputDetails.CacheWriteTokens, raw.InputDetails.CreatedCacheTokens, raw.CacheWriteTokens, raw.CreatedCacheTokens)
		u.CacheWrite5m = firstPositive(raw.InputDetails.CacheWriteDetails.TTL5m, raw.CacheWriteDetails.TTL5m)
		u.CacheWrite1h = firstPositive(raw.InputDetails.CacheWriteDetails.TTL1h, raw.CacheWriteDetails.TTL1h, raw.CacheWrite1hTokens)
	}
	if u.CompletionTokens == 0 {
		u.CompletionTokens = raw.OutputTokens
	}
	if u.OutputTokens == 0 {
		u.OutputTokens = raw.CompletionTokens
	}
	return nil
}

// ToolCall 是 OpenAI Chat Completions 的工具调用（流式分片会拼接 name/arguments）。
type ToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// CallResult 是一次请求的完整留档：传输层、三口径计时、内容与原文。
type CallResult struct {
	HTTPStatus int    `json:"http_status"`
	Endpoint   string `json:"endpoint,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	// Headers 保留请求头/响应头，便于排查 NewAPI 等上游的 request id、路由与限流信息。
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	// TTFT：请求发出 → 首个 SSE chunk；非流式时等于 E2E。
	TTFTMs int64 `json:"ttft_ms"`
	// TTFT-业务：请求发出 → 首个非空 delta.content（§2）。
	TTFTContentMs int64   `json:"ttft_content_ms"`
	E2EMs         int64   `json:"e2e_ms"`
	Chunks        int     `json:"chunks"`
	ITLMsa        []int64 `json:"itl_ms,omitempty"`
	// Chat/Responses 响应头中的缓存写入分档（Msh-Usage-Cache-Write-Tokens-5m/1h）。
	CacheWrite5m int64 `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1h int64 `json:"cache_write_1h_tokens,omitempty"`
	// ConnMs 是 DNS+TCP+TLS 建连耗时；复用连接时为 0 且 ConnReused=true。
	ConnMs     int64 `json:"conn_ms,omitempty"`
	ConnReused bool  `json:"conn_reused,omitempty"`
	// TTFBMs：请求发出 → 响应首字节（含排队与预填充开始）。
	TTFBMs int64 `json:"ttfb_ms,omitempty"`
	// MaxITLMs：本请求相邻 chunk 间隔的最大值（流式卡顿指标）。
	MaxITLMs     int64      `json:"max_itl_ms,omitempty"`
	Content      string     `json:"content"`
	Reasoning    string     `json:"reasoning,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        *Usage     `json:"usage,omitempty"`
	// Raw 是完整响应原文（SSE 或 JSON），超过 RawCap 截断并记录 RawSHA 全文哈希。
	Raw    string `json:"raw"`
	RawSHA string `json:"raw_sha"`
	// ErrKind：transport(连接/超时/断流) / ""(HTTP 层正常返回)。
	ErrKind string `json:"err_kind,omitempty"`
	ErrMsg  string `json:"err_msg,omitempty"`
}

const RawCap = 256 * 1024

// traceTiming 汇总 httptrace 的建连与首字节时间。
type traceTiming struct {
	start     time.Time
	dnsMs     int64
	connMs    int64
	tlsMs     int64
	ttfbMs    int64
	reused    bool
	dnsStart  time.Time
	connStart time.Time
	tlsStart  time.Time
}

// trace 返回挂在请求上的 httptrace 钩子；非流式请求同样用它测 TTFB。
func (t *traceTiming) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { t.dnsStart = time.Now() },
		DNSDone: func(httptrace.DNSDoneInfo) {
			if !t.dnsStart.IsZero() {
				t.dnsMs = time.Since(t.dnsStart).Milliseconds()
			}
		},
		ConnectStart: func(_, _ string) {
			if t.connStart.IsZero() {
				t.connStart = time.Now()
			}
		},
		ConnectDone: func(_, _ string, _ error) {
			if !t.connStart.IsZero() {
				t.connMs = time.Since(t.connStart).Milliseconds()
			}
		},
		TLSHandshakeStart: func() { t.tlsStart = time.Now() },
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			if !t.tlsStart.IsZero() {
				t.tlsMs = time.Since(t.tlsStart).Milliseconds()
			}
		},
		GotConn: func(info httptrace.GotConnInfo) { t.reused = info.Reused },
		GotFirstResponseByte: func() {
			t.ttfbMs = time.Since(t.start).Milliseconds()
		},
	}
}

// connMillis 是纯建连耗时（不含复用连接）。
func (t *traceTiming) connMillis() int64 {
	if t.reused {
		return 0
	}
	return t.dnsMs + t.connMs + t.tlsMs
}

// apply 把计时结果写入调用结果。
func (t *traceTiming) apply(r *CallResult) {
	r.ConnMs = t.connMillis()
	r.ConnReused = t.reused
	if t.ttfbMs > 0 {
		r.TTFBMs = t.ttfbMs
	}
}

// Client 是 OpenAI 兼容网关的最小客户端。
type Client struct {
	http *http.Client
	// OnRequest 可选：每发起一次上游请求调用一次（含重试），供实时进度展示。
	OnRequest func()
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
	if c.OnRequest != nil {
		c.OnRequest()
	}
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
	return c.doChat(ctx, t, bytes.NewReader(payload), int64(len(payload)), stream, timeout)
}

// ChatRaw 以流式 body 调用 /chat/completions；用于大 base64 视频，避免整包驻留内存。
func (c *Client) ChatRaw(ctx context.Context, t Target, body io.Reader, contentLength int64, stream bool, timeout time.Duration) CallResult {
	return c.doChat(ctx, t, body, contentLength, stream, timeout)
}

func (c *Client) doChat(ctx context.Context, t Target, body io.Reader, contentLength int64, stream bool, timeout time.Duration) CallResult {
	if c.OnRequest != nil {
		c.OnRequest()
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, strings.TrimSuffix(t.BaseURL, "/")+"/chat/completions", body)
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	start := time.Now()
	tm := &traceTiming{start: start}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), tm.trace()))
	res, err := c.http.Do(req)
	if err != nil {
		return CallResult{ErrKind: "transport", ErrMsg: err.Error(), E2EMs: ms(start)}
	}
	defer res.Body.Close()
	r := CallResult{HTTPStatus: res.StatusCode, Endpoint: "/chat/completions", RequestID: responseRequestID(res.Header)}
	attachHeaders(&r, req, res)
	captureCacheWriteHeaders(res.Header, &r)
	tm.apply(&r)
	if r.TTFBMs == 0 {
		r.TTFBMs = ms(start)
	}
	if stream && res.StatusCode == 200 {
		c.readSSE(reqCtx, res.Body, &r, start)
	} else {
		c.readJSON(res.Body, &r, start)
	}
	return r
}

func headerSnapshot(h http.Header, maskSecrets bool) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for key, values := range h {
		value := strings.Join(values, ", ")
		lower := strings.ToLower(key)
		if maskSecrets && (lower == "authorization" || lower == "x-api-key" || lower == "api-key" || lower == "proxy-authorization") {
			value = "***"
		}
		if !maskSecrets && lower == "set-cookie" {
			value = "***"
		}
		out[key] = value
	}
	return out
}

func responseRequestID(h http.Header) string {
	for _, key := range []string{
		"X-Request-Id", "Request-Id", "X-OneAPI-Request-Id", "X-NewAPI-Request-Id",
		"X-New-Api-Request-Id", "X-Trace-Id", "X-Upstream-Request-Id", "X-Ws-Request-Id", "CF-Ray",
	} {
		if value := strings.TrimSpace(h.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func attachHeaders(r *CallResult, req *http.Request, res *http.Response) {
	r.RequestHeaders = headerSnapshot(req.Header, true)
	if req.ContentLength > 0 {
		r.RequestHeaders["Content-Length"] = strconv.FormatInt(req.ContentLength, 10)
	}
	if res != nil {
		r.ResponseHeaders = headerSnapshot(res.Header, false)
	}
}

func captureCacheWriteHeaders(h http.Header, r *CallResult) {
	r.CacheWrite5m = int64(headerInt(h, "Msh-Usage-Cache-Write-Tokens-5m"))
	r.CacheWrite1h = int64(headerInt(h, "Msh-Usage-Cache-Write-Tokens-1h"))
}

func headerInt(h http.Header, key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(h.Get(key)))
	return n
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
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
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
		for _, call := range parsed.Choices[0].Message.ToolCalls {
			r.ToolCalls = append(r.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
		}
	}
	r.Usage = parsed.Usage
}

func (c *Client) readSSE(ctx context.Context, body io.Reader, r *CallResult, start time.Time) {
	var raw bytes.Buffer
	reader := bufio.NewReader(io.TeeReader(body, &raw))
	var content, reasoning strings.Builder
	var usage *Usage
	toolCalls := map[int]*ToolCall{}
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
							ToolCalls []struct {
								Index    int    `json:"index"`
								ID       string `json:"id"`
								Function struct {
									Name      string `json:"name"`
									Arguments string `json:"arguments"`
								} `json:"function"`
							} `json:"tool_calls"`
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
						for _, tc := range chunk.Choices[0].Delta.ToolCalls {
							item := toolCalls[tc.Index]
							if item == nil {
								item = &ToolCall{}
								toolCalls[tc.Index] = item
							}
							if tc.ID != "" {
								item.ID = tc.ID
							}
							item.Name += tc.Function.Name
							item.Arguments += tc.Function.Arguments
						}
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
	for _, gap := range r.ITLMsa {
		if gap > r.MaxITLMs {
			r.MaxITLMs = gap
		}
	}
	if len(toolCalls) > 0 {
		indexes := make([]int, 0, len(toolCalls))
		for index := range toolCalls {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			r.ToolCalls = append(r.ToolCalls, *toolCalls[index])
		}
	}
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

// Responses 调用官方 /v1/responses；请求体使用 input/max_output_tokens/prompt_cache_options。
func (c *Client) Responses(ctx context.Context, t Target, body map[string]any, timeout time.Duration) CallResult {
	if _, ok := body["model"]; !ok {
		body["model"] = t.Model
	}
	return c.postJSON(ctx, t, strings.TrimSuffix(t.BaseURL, "/")+"/responses", body, timeout, false)
}

// Messages 调用 Messages 兼容端点：优先 /v1/messages，失败再试 /anthropic/v1/messages。
// 对缓存用例，若优先端点返回 200 但没有缓存计量，也会继续尝试兼容端点。
func (c *Client) Messages(ctx context.Context, t Target, body map[string]any, timeout time.Duration) CallResult {
	if _, ok := body["model"]; !ok {
		body["model"] = t.Model
	}
	var best CallResult
	for i, url := range messagesURLs(t.BaseURL) {
		r := c.postJSON(ctx, t, url, body, timeout, true)
		if r.HTTPStatus == 200 {
			if i == 0 && !hasCacheUsage(r) {
				best = r
				continue
			}
			return r
		}
		if best.HTTPStatus == 0 {
			best = r
		}
	}
	return best
}

func hasCacheUsage(r CallResult) bool {
	return r.Usage != nil && (r.Usage.CachedTokens > 0 || r.Usage.CacheWriteTokens > 0 || r.Usage.CacheWrite1h > 0)
}

// messagesURLs 返回 Messages 兼容端点候选：优先用户要求的 /v1/messages，
// 再兼容官方的 /anthropic/v1/messages；测试只要任一通路可用即可。
func messagesURLs(base string) []string {
	base = strings.TrimSuffix(base, "/")
	if strings.HasSuffix(base, "/v1/messages") {
		return []string{base}
	}
	if strings.HasSuffix(base, "/anthropic/v1") {
		root := strings.TrimSuffix(base, "/anthropic/v1")
		return []string{root + "/v1/messages", base + "/messages"}
	}
	if strings.HasSuffix(base, "/anthropic") {
		root := strings.TrimSuffix(base, "/anthropic")
		return []string{root + "/v1/messages", base + "/v1/messages"}
	}
	if strings.HasSuffix(base, "/v1") {
		root := strings.TrimSuffix(base, "/v1")
		return []string{base + "/messages", root + "/anthropic/v1/messages"}
	}
	return []string{base + "/v1/messages", base + "/anthropic/v1/messages"}
}

func (c *Client) postJSON(ctx context.Context, t Target, url string, body map[string]any, timeout time.Duration, anthropic bool) CallResult {
	if c.OnRequest != nil {
		c.OnRequest()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	if anthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	start := time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		return CallResult{ErrKind: "transport", ErrMsg: err.Error(), E2EMs: ms(start)}
	}
	defer res.Body.Close()
	r := CallResult{HTTPStatus: res.StatusCode, Endpoint: url, RequestID: responseRequestID(res.Header)}
	attachHeaders(&r, req, res)
	captureCacheWriteHeaders(res.Header, &r)
	c.readJSON(res.Body, &r, start)
	return r
}

// ModelsCall 返回 /models 的完整证据（含请求头/响应头），供前置闸门留档。
func (c *Client) ModelsCall(ctx context.Context, t Target) CallResult {
	if c.OnRequest != nil {
		c.OnRequest()
	}
	reqCtx, cancel := context.WithTimeout(ctx, TimeoutNormal)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimSuffix(t.BaseURL, "/")+"/models", nil)
	if err != nil {
		return CallResult{ErrKind: "client", ErrMsg: err.Error()}
	}
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	start := time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		return CallResult{ErrKind: "transport", ErrMsg: err.Error(), E2EMs: ms(start)}
	}
	defer res.Body.Close()
	r := CallResult{HTTPStatus: res.StatusCode, Endpoint: "/models", RequestID: responseRequestID(res.Header)}
	attachHeaders(&r, req, res)
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	r.E2EMs, r.TTFTMs, r.TTFTContentMs = ms(start), ms(start), ms(start)
	finalizeRaw(&r, body)
	if err != nil {
		r.ErrKind, r.ErrMsg = "transport", err.Error()
	}
	return r
}

// Models 校验 /v1/models 中目标模型在列（前置闸门 G1）。
func (c *Client) Models(ctx context.Context, t Target) (int, []string, error) {
	r := c.ModelsCall(ctx, t)
	if r.HTTPStatus != 200 || r.ErrKind != "" {
		return r.HTTPStatus, nil, errors.New(r.ErrMsg)
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Raw), &parsed) != nil {
		return r.HTTPStatus, nil, fmt.Errorf("models 响应解析失败")
	}
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		ids = append(ids, m.ID)
	}
	return r.HTTPStatus, ids, nil
}
