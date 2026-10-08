package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"nicetools/backend/internal/config"
	"nicetools/backend/internal/model"
	"nicetools/backend/internal/service"
	"nicetools/backend/internal/store"
)

const adminPassword = "CorrectHorseBattery12!"

type fixture struct {
	t      *testing.T
	api    *API
	router http.Handler
	config config.Config
	store  *store.Store
}
type client struct {
	f       *fixture
	cookies map[string]string
	csrf    string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	static := filepath.Join(root, "static")
	if err := os.MkdirAll(filepath.Join(static, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"index.html": "<html>application</html>", "tool-assets.json": `{"assets/tool.js":["text-delimiter"]}`, "assets/tool.js": "export const tool = 'text';", "assets/public.js": "export const shared = true;", "assets/tool.js.map": "private"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(static, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	catalog := filepath.Join(root, "catalog.json")
	if err := os.WriteFile(catalog, []byte(`[{"id":"text-delimiter","name":"Text","description":"d","emoji":"x","category":"text","tags":[]},{"id":"future-tool","name":"Future","description":"d","emoji":"x","category":"text","tags":[]}]`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Port: "8080", DBPath: filepath.Join(root, "db.sqlite"), StaticDir: static, CatalogPath: catalog, AdminUsername: "admin", AdminPassword: adminPassword}
	db, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(catalog, "admin", adminPassword); err != nil {
		t.Fatal(err)
	}
	api := New(service.New(db), cfg)
	if err := api.Err(); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, api: api, router: api.Run(), config: cfg, store: db}
	t.Cleanup(func() { _ = db.Close() })
	return f
}
func (f *fixture) client() *client { return &client{f: f, cookies: map[string]string{}} }
func (c *client) request(method, path string, body interface{}, csrf bool) *httptest.ResponseRecorder {
	c.f.t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			c.f.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(data))
	req.Header.Set("Origin", "http://example.test")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range c.cookies {
		req.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	if csrf {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	res := httptest.NewRecorder()
	c.f.router.ServeHTTP(res, req)
	for _, cookie := range res.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
	var session struct {
		Token string `json:"csrf_token"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &session)
	if session.Token != "" {
		c.csrf = session.Token
	}
	return res
}
func expect(t *testing.T, res *httptest.ResponseRecorder, status int) {
	t.Helper()
	if res.Code != status {
		t.Fatalf("status=%d want=%d body=%s", res.Code, status, res.Body.String())
	}
}
func decode[T any](t *testing.T, res *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(res.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func (c *client) login(username, password string) *model.User {
	c.f.t.Helper()
	expect(c.f.t, c.request("GET", "/api/session", nil, false), 200)
	res := c.request("POST", "/api/auth/login", map[string]string{"username": username, "password": password}, true)
	expect(c.f.t, res, 200)
	return decode[struct {
		User *model.User `json:"user"`
	}](c.f.t, res).User
}
func (f *fixture) admin() *client { c := f.client(); c.login("admin", adminPassword); return c }
func itoa(id uint) string         { return strconv.FormatUint(uint64(id), 10) }
func save(t *testing.T, c *client, input service.PolicyInput) {
	t.Helper()
	expect(t, c.request("PUT", "/api/admin/tools/text-delimiter", input, true), 200)
}
func createGroup(t *testing.T, c *client, name string) model.Group {
	t.Helper()
	res := c.request("POST", "/api/admin/groups", service.GroupInput{Name: name}, true)
	expect(t, res, 201)
	return decode[model.Group](t, res)
}
func createUser(t *testing.T, c *client, name, role string, groups []uint) model.User {
	t.Helper()
	res := c.request("POST", "/api/admin/users", service.UserInput{Username: name, DisplayName: name, Password: adminPassword, Role: role, GroupIDs: groups}, true)
	expect(t, res, 201)
	return decode[model.User](t, res)
}

func TestNoRegistrationAndAdminIsolation(t *testing.T) {
	f := setup(t)
	guest := f.client()
	expect(t, guest.request("GET", "/api/admin/users", nil, false), 401)
	expect(t, guest.request("POST", "/api/auth/register", map[string]string{"username": "intruder", "password": adminPassword}, true), 404)
	admin := f.admin()
	user := createUser(t, admin, "member", "user", nil)
	member := f.client()
	member.login(user.Username, adminPassword)
	expect(t, member.request("GET", "/api/admin/users", nil, false), 403)
	expect(t, member.request("POST", "/api/admin/groups", service.GroupInput{Name: "unauthorized"}, true), 403)
}
func TestCSRFAndSessionRotation(t *testing.T) {
	f := setup(t)
	c := f.client()
	expect(t, c.request("GET", "/api/session", nil, false), 200)
	expect(t, c.request("POST", "/api/auth/login", map[string]string{"username": "admin", "password": adminPassword}, false), 403)
	old := c.cookies["session"]
	c.login("admin", adminPassword)
	stolen := f.client()
	stolen.cookies["session"] = old
	expect(t, stolen.request("GET", "/api/admin/users", nil, false), 401)
	expect(t, c.request("POST", "/api/auth/logout", map[string]string{}, false), 403)
	old = c.cookies["session"]
	expect(t, c.request("POST", "/api/auth/logout", map[string]string{}, true), 200)
	stolen.cookies["session"] = old
	expect(t, stolen.request("GET", "/api/admin/users", nil, false), 401)
	expect(t, c.request("GET", "/api/admin/users", nil, false), 401)
}
func TestCrossOriginMutationRejected(t *testing.T) {
	f := setup(t)
	c := f.admin()
	req := httptest.NewRequest("POST", "http://example.test/api/admin/groups", bytes.NewBufferString(`{"name":"bad"}`))
	req.Header.Set("Origin", "http://attacker.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	for key, value := range c.cookies {
		req.AddCookie(&http.Cookie{Name: key, Value: value})
	}
	res := httptest.NewRecorder()
	f.router.ServeHTTP(res, req)
	expect(t, res, 403)
}
func TestToolVisibilityUseAndProtectedAssets(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	guest := f.client()
	policy := service.PolicyInput{Enabled: true, GuestVisible: false, GuestUse: false, MemberVisible: false, MemberUse: false}
	save(t, admin, policy)
	expect(t, guest.request("GET", "/api/tools/text-delimiter/access", nil, false), 404)
	expect(t, guest.request("GET", "/assets/tool.js", nil, false), 403)
	policy.GuestVisible = true
	save(t, admin, policy)
	expect(t, guest.request("GET", "/api/tools/text-delimiter/access", nil, false), 403)
	rows := decode[[]service.ToolAccess](t, guest.request("GET", "/api/tools", nil, false))
	if len(rows) != 1 || rows[0].CanUse {
		t.Fatalf("visible-but-locked list=%+v", rows)
	}
	policy.GuestUse = true
	save(t, admin, policy)
	expect(t, guest.request("GET", "/api/tools/text-delimiter/access", nil, false), 200)
	asset := guest.request("GET", "/assets/tool.js", nil, false)
	expect(t, asset, 200)
	if asset.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("protected JS must not be cached")
	}
	policy.Enabled = false
	save(t, admin, policy)
	expect(t, guest.request("GET", "/api/tools/text-delimiter/access", nil, false), 404)
	expect(t, admin.request("GET", "/api/tools/text-delimiter/access", nil, false), 403)
	expect(t, guest.request("GET", "/tool-assets.json", nil, false), 404)
	expect(t, guest.request("GET", "/assets/tool.js.map", nil, false), 404)
	expect(t, guest.request("GET", "/assets/missing.js", nil, false), 404)
	expect(t, guest.request("GET", "/api/not-found", nil, false), 404)
	expect(t, guest.request("GET", "/admin", nil, false), 200)
}
func TestGroupDenyPrecedenceAndDeletionFallback(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	allow := createGroup(t, admin, "allow")
	deny := createGroup(t, admin, "deny")
	user := createUser(t, admin, "multiple-groups", "user", []uint{allow.ID, deny.ID})
	member := f.client()
	member.login(user.Username, adminPassword)
	policy := service.PolicyInput{Enabled: true, GroupRules: []service.RuleInput{{GroupID: allow.ID, Visible: new(true), Use: new(true)}}}
	save(t, admin, policy)
	expect(t, member.request("GET", "/api/tools/text-delimiter/access", nil, false), 200)
	policy.GroupRules = append(policy.GroupRules, service.RuleInput{GroupID: deny.ID, Visible: new(true), Use: new(false)})
	save(t, admin, policy)
	expect(t, member.request("GET", "/api/tools/text-delimiter/access", nil, false), 403)
	policy.GroupRules[1].Visible = new(false)
	policy.GroupRules[1].Use = nil
	save(t, admin, policy)
	expect(t, member.request("GET", "/api/tools/text-delimiter/access", nil, false), 404)
	expect(t, admin.request("DELETE", "/api/admin/groups/"+itoa(deny.ID), nil, true), 204)
	expect(t, member.request("GET", "/api/tools/text-delimiter/access", nil, false), 200)
	expect(t, admin.request("DELETE", "/api/admin/groups/"+itoa(allow.ID), nil, true), 204)
	expect(t, member.request("GET", "/api/tools/text-delimiter/access", nil, false), 404)
}
func TestLastAdminAndSelfDeleteProtected(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	users := decode[[]model.User](t, admin.request("GET", "/api/admin/users", nil, false))
	id := users[0].ID
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(id), service.UserInput{Role: "user", DisplayName: "admin"}, true), 409)
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(id), service.UserInput{Role: "admin", Disabled: true}, true), 409)
	expect(t, admin.request("DELETE", "/api/admin/users/"+itoa(id), nil, true), 409)
	second := createUser(t, admin, "second-admin", "admin", nil)
	other := f.client()
	other.login(second.Username, adminPassword)
	expect(t, other.request("DELETE", "/api/admin/users/"+itoa(id), nil, true), 204)
	expect(t, admin.request("GET", "/api/admin/users", nil, false), 401)
	expect(t, other.request("PUT", "/api/admin/users/"+itoa(second.ID), service.UserInput{Role: "user"}, true), 409)
}
func TestPasswordResetDisableAndRoleRevocation(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	user := createUser(t, admin, "revoked-user", "user", nil)
	member := f.client()
	member.login(user.Username, adminPassword)
	input := service.UserInput{Role: "user", Password: "UpdatedPassword-2026"}
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(user.ID), input, true), 200)
	session := decode[struct {
		User *model.User `json:"user"`
	}](t, member.request("GET", "/api/session", nil, false))
	if session.User != nil {
		t.Fatal("password reset kept session")
	}
	member.login(user.Username, input.Password)
	expect(t, member.request("PUT", "/api/account/password", map[string]string{"current_password": input.Password, "password": "SelfChangedPass-2026"}, true), 204)
	expect(t, member.request("PUT", "/api/account/password", map[string]string{}, true), 403)
	member.login(user.Username, "SelfChangedPass-2026")
	input.Password = ""
	input.Disabled = true
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(user.ID), input, true), 200)
	session = decode[struct {
		User *model.User `json:"user"`
	}](t, member.request("GET", "/api/session", nil, false))
	if session.User != nil {
		t.Fatal("disable kept session")
	}
	input.Disabled = false
	input.Role = "admin"
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(user.ID), input, true), 200)
	member.login(user.Username, "SelfChangedPass-2026")
	input.Role = "user"
	expect(t, admin.request("PUT", "/api/admin/users/"+itoa(user.ID), input, true), 200)
	expect(t, member.request("GET", "/api/admin/users", nil, false), 401)
}
func TestPolicyValidationAndDuplicateUser(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	createUser(t, admin, "duplicate", "user", nil)
	expect(t, admin.request("POST", "/api/admin/users", service.UserInput{Username: "duplicate", Password: adminPassword, Role: "user"}, true), 409)
	expect(t, admin.request("PUT", "/api/admin/tools/text-delimiter", service.PolicyInput{Enabled: true, GuestUse: true}, true), 400)
	expect(t, admin.request("PUT", "/api/admin/tools/text-delimiter", service.PolicyInput{Enabled: true, GroupRules: []service.RuleInput{{GroupID: 999, Visible: new(true), Use: new(true)}}}, true), 400)
	expect(t, admin.request("POST", "/api/admin/users", service.UserInput{Username: "bad-groups", Password: adminPassword, Role: "user", GroupIDs: []uint{999}}, true), 400)
}
func TestMissingManifestRefusesServing(t *testing.T) {
	f := setup(t)
	if err := os.Remove(filepath.Join(f.config.StaticDir, "tool-assets.json")); err != nil {
		t.Fatal(err)
	}
	api := New(service.New(f.store), f.config)
	if api.Err() == nil {
		t.Fatal("missing manifest must fail startup")
	}
	req := httptest.NewRequest("GET", "http://example.test/", nil)
	res := httptest.NewRecorder()
	api.Run().ServeHTTP(res, req)
	expect(t, res, 503)
}
func TestPolicySurvivesCatalogSync(t *testing.T) {
	f := setup(t)
	admin := f.admin()
	policy := service.PolicyInput{Enabled: true, GuestVisible: true, GuestUse: false}
	save(t, admin, policy)
	if err := f.store.Sync(f.config.CatalogPath, "admin", ""); err != nil {
		t.Fatal(err)
	}
	expect(t, f.client().request("GET", "/api/tools/text-delimiter/access", nil, false), 403)
	expect(t, f.client().request("GET", "/api/tools/future-tool/access", nil, false), 404)
}
