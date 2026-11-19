package k3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nicetools/backend/internal/tools/llmadmit/suite"
)

func TestVideoLimitBase64ProbeStreamsBody(t *testing.T) {
	var bodyLen int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		bodyLen = int64(len(body))
		if r.ContentLength != bodyLen {
			t.Errorf("content-length=%d actual=%d", r.ContentLength, bodyLen)
		}
		if !strings.Contains(string(body), `"video_url"`) || !strings.Contains(string(body), `base64,`) {
			t.Errorf("body missing video_url/base64")
		}
		if !strings.Contains(string(body), `"model":"kimi-k3"`) {
			t.Errorf("body missing model")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"红色","reasoning_content":"红"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer server.Close()
	c := &suite.CaseCtx{
		Context: context.Background(),
		Target:  suite.Target{BaseURL: server.URL, APIKey: "sk-test", Model: "kimi-k3"},
		Client:  suite.NewClient(),
	}
	r, requestBytes, _ := videoLimitBase64Probe(c, "base64", 1<<20)
	if r.HTTPStatus != 200 || !strings.Contains(r.Content, "红") {
		t.Fatalf("result=%d content=%q err=%s", r.HTTPStatus, r.Content, r.ErrMsg)
	}
	if requestBytes != int(bodyLen) {
		t.Fatalf("requestBytes=%d bodyLen=%d", requestBytes, bodyLen)
	}
	if bodyLen <= 1<<20 {
		t.Fatalf("body too small: %d", bodyLen)
	}
}
