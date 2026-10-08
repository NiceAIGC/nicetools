package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"nicetools/backend/internal/config"
	"nicetools/backend/internal/model"
	"nicetools/backend/internal/service"
)

type API struct {
	S          *service.Service
	C          config.Config
	assets     map[string][]string
	staticRoot string
	initErr    error
	mu         sync.Mutex
	attempts   map[string]attempt
}
type attempt struct {
	count   int
	expires time.Time
}

func New(s *service.Service, c config.Config) *API {
	a := &API{S: s, C: c, assets: map[string][]string{}, attempts: map[string]attempt{}}
	a.staticRoot, a.initErr = filepath.Abs(c.StaticDir)
	if a.initErr != nil {
		return a
	}
	data, err := os.ReadFile(filepath.Join(a.staticRoot, "tool-assets.json"))
	if err != nil {
		a.initErr = errors.New("static tool-assets.json missing; run pnpm build")
		return a
	}
	if err = json.Unmarshal(data, &a.assets); err != nil || a.assets == nil {
		a.initErr = errors.New("invalid tool-assets.json")
		return a
	}
	for asset, ids := range a.assets {
		if !strings.HasPrefix(asset, "assets/") || path.Clean(asset) != asset || path.Ext(asset) != ".js" || len(ids) == 0 {
			a.initErr = errors.New("invalid protected asset entry")
			return a
		}
		info, err := os.Stat(filepath.Join(a.staticRoot, filepath.FromSlash(asset)))
		if err != nil || !info.Mode().IsRegular() {
			a.initErr = errors.New("protected asset missing")
			return a
		}
	}
	if info, err := os.Stat(filepath.Join(a.staticRoot, "index.html")); err != nil || !info.Mode().IsRegular() {
		a.initErr = errors.New("frontend index.html missing")
	}
	return a
}
func (a *API) Err() error { return a.initErr }
func (a *API) Run() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), a.headers)
	_ = r.SetTrustedProxies(nil)
	r.GET("/api/health", a.health)
	r.GET("/api/session", a.session)
	r.GET("/api/tools", a.tools)
	r.GET("/api/tools/:id/access", a.access)
	mutations := r.Group("/api", a.csrf)
	mutations.POST("/auth/login", a.login)
	mutations.POST("/auth/logout", a.logout)
	mutations.PUT("/account/password", a.requireAuth, a.password)
	admin := r.Group("/api/admin", a.requireAuth, a.requireAdmin)
	admin.GET("/users", a.users)
	admin.GET("/groups", a.groups)
	admin.GET("/tools", a.policies)
	admin.GET("/audit", a.audit)
	write := admin.Group("", a.csrf)
	write.POST("/users", a.createUser)
	write.PUT("/users/:id", a.updateUser)
	write.DELETE("/users/:id", a.deleteUser)
	write.POST("/groups", a.createGroup)
	write.PUT("/groups/:id", a.updateGroup)
	write.DELETE("/groups/:id", a.deleteGroup)
	write.PUT("/tools/:id", a.savePolicy)
	r.NoRoute(a.static)
	return r
}
func (a *API) headers(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	if strings.HasPrefix(c.Request.URL.Path, "/api") {
		c.Header("Cache-Control", "no-store")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if a.initErr != nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "前端构建未就绪"})
		return
	}
	c.Next()
}
func (a *API) fail(c *gin.Context, err error) {
	var problem *service.Error
	if errors.As(err, &problem) {
		c.AbortWithStatusJSON(problem.Status, gin.H{"error": problem.Message})
		return
	}
	log.Printf("request %s %s: %v", c.Request.Method, c.FullPath(), err)
	c.AbortWithStatusJSON(500, gin.H{"error": "服务暂时不可用"})
}
func reject(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
func token(c *gin.Context) string { value, _ := c.Cookie("session"); return value }
func (a *API) user(c *gin.Context) (*model.User, error) {
	if token(c) == "" {
		return nil, nil
	}
	user, _, err := a.S.Session(token(c))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return user, err
}
func (a *API) requireAuth(c *gin.Context) {
	user, err := a.user(c)
	if err != nil {
		a.fail(c, err)
		return
	}
	if user == nil {
		reject(c, 401, "请先登录")
		return
	}
	c.Set("user", user)
	c.Next()
}
func (a *API) requireAdmin(c *gin.Context) {
	if current(c).Role != "admin" {
		reject(c, 403, "需要管理员权限")
		return
	}
	c.Next()
}
func current(c *gin.Context) *model.User { return c.MustGet("user").(*model.User) }
func (a *API) csrf(c *gin.Context) {
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		reject(c, 403, "跨站请求被拒绝")
		return
	}
	origin := c.GetHeader("Origin")
	if origin != "" {
		parsed, err := url.Parse(origin)
		allowed := false
		if err == nil && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Path == "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			if a.C.PublicOrigin != "" {
				allowed = origin == a.C.PublicOrigin
			} else {
				scheme := "http"
				if c.Request.TLS != nil {
					scheme = "https"
				}
				allowed = parsed.Scheme == scheme && strings.EqualFold(parsed.Host, c.Request.Host)
			}
		}
		if !allowed {
			reject(c, 403, "请求来源不匹配")
			return
		}
	}
	cookie, _ := c.Cookie("csrf")
	header := c.GetHeader("X-CSRF-Token")
	if len(cookie) != 64 || subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 || !a.S.CheckCSRF(token(c), header) {
		reject(c, 403, "会话校验失败，请刷新页面后重试")
		return
	}
	c.Next()
}
func (a *API) cookies(c *gin.Context, session, csrf string) {
	for _, pair := range [][2]string{{"session", session}, {"csrf", csrf}} {
		http.SetCookie(c.Writer, &http.Cookie{Name: pair[0], Value: pair[1], Path: "/", MaxAge: 604800, Expires: time.Now().Add(7 * 24 * time.Hour), HttpOnly: true, Secure: a.C.CookieSecure, SameSite: http.SameSiteLaxMode})
	}
}
func (a *API) clearCookies(c *gin.Context) {
	for _, name := range []string{"session", "csrf"} {
		http.SetCookie(c.Writer, &http.Cookie{Name: name, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.C.CookieSecure, SameSite: http.SameSiteLaxMode})
	}
}
func (a *API) session(c *gin.Context) {
	previous := token(c)
	csrf, _ := c.Cookie("csrf")
	user, _, err := a.S.Session(previous)
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !a.S.CheckCSRF(previous, csrf) {
		session, newCSRF, e := a.S.Issue(nil, previous)
		if e != nil {
			a.fail(c, e)
			return
		}
		a.cookies(c, session, newCSRF)
		c.JSON(200, gin.H{"user": nil, "csrf_token": newCSRF})
		return
	}
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"user": user, "csrf_token": csrf})
}
func (a *API) allowLogin(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for key, entry := range a.attempts {
		if now.After(entry.expires) {
			delete(a.attempts, key)
		}
	}
	entry := a.attempts[ip]
	if entry.expires.IsZero() {
		if len(a.attempts) >= 10000 {
			return false
		}
		entry.expires = now.Add(15 * time.Minute)
	}
	if entry.count >= 20 {
		return false
	}
	entry.count++
	a.attempts[ip] = entry
	return true
}
func (a *API) login(c *gin.Context) {
	if !a.allowLogin(c.ClientIP()) {
		c.Header("Retry-After", "900")
		reject(c, 429, "登录请求过多，请稍后重试")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bind(c, &input) {
		return
	}
	if len(input.Username) > 32 || len(input.Password) > 72 {
		reject(c, 401, "用户名或密码错误")
		return
	}
	user, session, csrf, err := a.S.Login(input.Username, input.Password, token(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.cookies(c, session, csrf)
	c.JSON(200, gin.H{"user": user, "csrf_token": csrf})
}
func (a *API) logout(c *gin.Context) {
	session, csrf, err := a.S.Logout(token(c))
	if err != nil {
		a.fail(c, err)
		return
	}
	a.cookies(c, session, csrf)
	c.JSON(200, gin.H{"user": nil, "csrf_token": csrf})
}
func (a *API) health(c *gin.Context) {
	db, err := a.S.S.DB.DB()
	if err == nil {
		err = db.PingContext(c.Request.Context())
	}
	if err != nil {
		reject(c, 503, "数据库不可用")
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
func bind(c *gin.Context, input interface{}) bool {
	if err := c.ShouldBindJSON(input); err != nil {
		reject(c, 400, "请求 JSON 无效")
		return false
	}
	return true
}
func numericID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		reject(c, 400, "记录 ID 无效")
		return 0, false
	}
	return uint(id), true
}
func (a *API) tools(c *gin.Context) {
	user, err := a.user(c)
	if err != nil {
		a.fail(c, err)
		return
	}
	tools, err := a.S.Tools(user)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, tools)
}
func (a *API) access(c *gin.Context) {
	user, err := a.user(c)
	if err != nil {
		a.fail(c, err)
		return
	}
	tool, err := a.S.Access(c.Param("id"), user)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, tool)
}
func (a *API) static(c *gin.Context) {
	raw := c.Request.URL.Path
	if strings.HasPrefix(raw, "/api/") || raw == "/api" {
		reject(c, 404, "接口不存在")
		return
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		reject(c, 404, "资源不存在")
		return
	}
	cleaned := path.Clean(raw)
	if raw != cleaned && raw != "/" {
		reject(c, 404, "资源不存在")
		return
	}
	relative := strings.TrimPrefix(cleaned, "/")
	if relative == "tool-assets.json" || strings.HasSuffix(relative, ".map") {
		reject(c, 404, "资源不存在")
		return
	}
	if ids, guarded := a.assets[relative]; guarded {
		c.Header("Cache-Control", "no-store")
		user, err := a.user(c)
		if err != nil {
			a.fail(c, err)
			return
		}
		permitted := false
		for _, id := range ids {
			if _, err := a.S.Access(id, user); err == nil {
				permitted = true
				break
			} else {
				var problem *service.Error
				if !errors.As(err, &problem) || (problem.Status != 403 && problem.Status != 404) {
					a.fail(c, err)
					return
				}
			}
		}
		if !permitted {
			reject(c, 403, "没有工具资源访问权限")
			return
		}
	}
	file := filepath.Join(a.staticRoot, filepath.FromSlash(relative))
	if path.Ext(relative) == "" {
		if strings.HasPrefix(relative, "assets/") {
			reject(c, 404, "资源不存在")
			return
		}
		file = filepath.Join(a.staticRoot, "index.html")
		c.Header("Cache-Control", "no-cache")
	} else if relative == "index.html" {
		c.Header("Cache-Control", "no-cache")
	}
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil || resolved != a.staticRoot && !strings.HasPrefix(resolved, a.staticRoot+string(os.PathSeparator)) {
		reject(c, 404, "资源不存在")
		return
	}
	f, err := os.Open(resolved)
	if err != nil {
		reject(c, 404, "资源不存在")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		reject(c, 404, "资源不存在")
		return
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
}
