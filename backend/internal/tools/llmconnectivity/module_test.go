package llmconnectivity

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nicetools/backend/internal/model"
)

func proxyContext(t *testing.T, payload string, user *model.User) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/proxy", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	if user != nil {
		c.Set("user", user)
	}
	return c, w
}

func TestProxyForwardsAndStripsBrowserSecrets(t *testing.T) {
	var gotAuth, gotCookie, gotTest string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		gotTest = r.Header.Get("X-Test")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "upstream=secret")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	payload := fmt.Sprintf(`{"url":%q,"method":"POST","headers":{"authorization":"Bearer key","cookie":"session=secret","x-test":"1"},"body":"{\"a\":1}","timeout_ms":5000}`, upstream.URL)
	c, w := proxyContext(t, payload, &model.User{Role: "admin"})
	New().proxy(c)

	if w.Code != http.StatusCreated || w.Body.String() != `{"ok":true}` {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
	if gotAuth != "Bearer key" || gotTest != "1" {
		t.Fatalf("forwarded headers auth=%q test=%q", gotAuth, gotTest)
	}
	if gotCookie != "" {
		t.Fatal("browser cookie leaked to upstream")
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("upstream Set-Cookie leaked to browser")
	}
}

func TestProxyBlocksPrivateHostForNonAdmin(t *testing.T) {
	payload := `{"url":"http://127.0.0.1:1234/v1/models","method":"GET","headers":{}}`
	c, w := proxyContext(t, payload, &model.User{Role: "user"})
	New().proxy(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}
