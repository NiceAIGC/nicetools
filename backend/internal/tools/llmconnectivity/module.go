// Package llmconnectivity 为大模型连通性测试提供同源代理：
// 浏览器直连第三方 API 常被 CORS 拦截，改由后端转发，密钥不落库、不写日志。
package llmconnectivity

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nicetools/backend/internal/module"
	"nicetools/backend/internal/service"
)

type Module struct{}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return "llm-connectivity" }

func (m *Module) Migrate(db *gorm.DB) error { return nil }

func (m *Module) Register(read, write, pub *gin.RouterGroup, d module.Deps) {
	write.POST("/proxy", m.proxy)
}

type proxyInput struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	TimeoutMS int               `json:"timeout_ms"`
}

var hopHeaders = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true,
	"proxy-authorization": true, "te": true, "trailer": true,
	"transfer-encoding": true, "upgrade": true,
}

// proxy 把一次连通性请求转发到目标接口，并把上游状态码/响应体原样流回浏览器。
// 只允许 GET/POST；非管理员禁止访问内网、回环与云元数据地址，避免 SSRF。
func (m *Module) proxy(c *gin.Context) {
	var input proxyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		module.Fail(c, service.Problem(400, "代理请求 JSON 无效"))
		return
	}
	target, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		module.Fail(c, service.Problem(400, "目标 URL 无效"))
		return
	}
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodGet && method != http.MethodPost {
		module.Fail(c, service.Problem(400, "代理仅支持 GET/POST"))
		return
	}
	if !isAdmin(c) && blockedHost(target.Hostname()) {
		module.Fail(c, service.Problem(400, "目标地址不可用于服务端代理"))
		return
	}
	timeout := time.Duration(input.TimeoutMS) * time.Millisecond
	if timeout < time.Second {
		timeout = time.Second
	}
	if timeout > 3*time.Minute {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	var body io.Reader
	if input.Body != "" {
		body = strings.NewReader(input.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		module.Fail(c, service.Problem(400, "目标请求构造失败"))
		return
	}
	for key, value := range input.Headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		// 禁止把浏览器会话 cookie / 代理相关的头带到上游。
		if lower == "" || lower == "cookie" || lower == "origin" || lower == "referer" ||
			strings.HasPrefix(lower, "sec-fetch-") || lower == "accept-encoding" ||
			hopHeaders[lower] || lower == "host" || lower == "content-length" {
			continue
		}
		req.Header.Set(key, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "NiceTools-Connectivity/1.0")
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "上游请求失败，请检查 Base URL 或网络"})
		return
	}
	defer res.Body.Close()
	for key, values := range res.Header {
		lower := strings.ToLower(key)
		if hopHeaders[lower] || lower == "content-length" || lower == "set-cookie" || lower == "content-encoding" {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(res.StatusCode)
	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func isAdmin(c *gin.Context) bool {
	user := module.CurrentUser(c)
	return user != nil && user.Role == "admin" && !user.Disabled
}

func blockedHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsMulticast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}
