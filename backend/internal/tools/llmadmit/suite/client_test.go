package suite

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUsageParsesAllCacheFormats(t *testing.T) {
	cases := []struct {
		name, raw                      string
		prompt, cached, write, write1h int
	}{
		{"chat write", `{"prompt_tokens":2132,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":2048}}`, 2132, 0, 2048, 0},
		{"chat hit", `{"prompt_tokens":2132,"prompt_tokens_details":{"cached_tokens":2048}}`, 2132, 2048, 0, 0},
		{"chat top-level write", `{"prompt_tokens":2132,"created_cache_tokens":2048,"cache_write_tokens":2048,"prompt_tokens_details":{"cached_tokens":0}}`, 2132, 0, 2048, 0},
		{"chat top-level ttl", `{"prompt_tokens":2132,"cache_write_tokens":2048,"cache_write_tokens_details":{"ttl_5m_tokens":0,"ttl_1h_tokens":2048}}`, 2132, 0, 2048, 2048},
		{"responses write", `{"input_tokens":2131,"output_tokens":16,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":2048}}`, 2131, 0, 2048, 0},
		{"responses hit", `{"input_tokens":2131,"input_tokens_details":{"cached_tokens":2048}}`, 2131, 2048, 0, 0},
		{"messages write 1h", `{"input_tokens":83,"output_tokens":8,"cache_creation_input_tokens":2048,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":2048}}`, 2131, 0, 2048, 2048},
		{"messages hit", `{"input_tokens":83,"output_tokens":8,"cache_read_input_tokens":2048}`, 2131, 2048, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var u Usage
			if err := json.Unmarshal([]byte(tc.raw), &u); err != nil {
				t.Fatal(err)
			}
			if u.PromptTokens != tc.prompt || u.CachedTokens != tc.cached || u.CacheWriteTokens != tc.write || u.CacheWrite1h != tc.write1h {
				t.Fatalf("usage=%+v want prompt=%d cached=%d write=%d 1h=%d", u, tc.prompt, tc.cached, tc.write, tc.write1h)
			}
		})
	}
}

func TestMessagesURLs(t *testing.T) {
	cases := []struct {
		base string
		want []string
	}{
		{"https://api.moonshot.cn/v1", []string{"https://api.moonshot.cn/v1/messages", "https://api.moonshot.cn/anthropic/v1/messages"}},
		{"https://api.moonshot.cn", []string{"https://api.moonshot.cn/v1/messages", "https://api.moonshot.cn/anthropic/v1/messages"}},
		{"https://api.moonshot.cn/anthropic", []string{"https://api.moonshot.cn/v1/messages", "https://api.moonshot.cn/anthropic/v1/messages"}},
		{"https://api.moonshot.cn/anthropic/v1", []string{"https://api.moonshot.cn/v1/messages", "https://api.moonshot.cn/anthropic/v1/messages"}},
	}
	for _, tc := range cases {
		got := messagesURLs(tc.base)
		if len(got) != len(tc.want) || got[0] != tc.want[0] || got[1] != tc.want[1] {
			t.Errorf("messagesURLs(%q)=%v want %v", tc.base, got, tc.want)
		}
	}
}

func TestCacheWriteHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Msh-Usage-Cache-Write-Tokens-5m", "0")
	h.Set("Msh-Usage-Cache-Write-Tokens-1h", "2048")
	var r CallResult
	captureCacheWriteHeaders(h, &r)
	if r.CacheWrite1h != 2048 || r.CacheWrite5m != 0 {
		t.Fatalf("headers=%+v", r)
	}
}

func TestReadJSONToolCalls(t *testing.T) {
	raw := `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":"tool_calls"}]}`
	r := CallResult{HTTPStatus: 200}
	c := &Client{}
	c.readJSON(strings.NewReader(raw), &r, time.Now())
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].ID != "call_1" || r.ToolCalls[0].Name != "get_weather" || r.ToolCalls[0].Arguments != `{"city":"Beijing"}` {
		t.Fatalf("tool_calls=%+v", r.ToolCalls)
	}
}

func TestReadSSEToolCalls(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"city\\\":\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"Beijing\\\"}\"}}]}}]}\n\n" +
		"data: [DONE]\n\n"
	r := CallResult{HTTPStatus: 200}
	c := &Client{}
	c.readSSE(context.Background(), strings.NewReader(raw), &r, time.Now())
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].ID != "call_1" || r.ToolCalls[0].Name != "get_weather" || r.ToolCalls[0].Arguments != `{"city":"Beijing"}` {
		t.Fatalf("tool_calls=%+v", r.ToolCalls)
	}
}
