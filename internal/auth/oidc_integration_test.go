package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"bootimus/internal/auth"
	"bootimus/internal/models"
	"bootimus/internal/storage"
)

const integrationEnvVar = "BOOTIMUS_TEST_OIDC_URL"

type integrationEnv struct {
	server *httptest.Server
	store  *storage.SQLiteStore
	auth   *auth.Manager
}

func newIntegrationEnv(t *testing.T, cfg auth.OIDCConfig) *integrationEnv {
	t.Helper()
	base := os.Getenv(integrationEnvVar)
	if base == "" {
		t.Skipf("%s not set; run `make test-integration` to start a throwaway OIDC provider", integrationEnvVar)
	}

	store, err := storage.NewSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AutoMigrate(); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	if cfg.Issuer == "" {
		cfg.Issuer = strings.TrimRight(base, "/") + "/bootimus"
	}
	cfg.ClientID = "bootimus"
	cfg.ClientSecret = "bootimus-secret"
	cfg.RedirectURL = server.URL + "/api/oidc/callback"
	if cfg.Scopes == nil {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}

	mgr, err := auth.NewManager(store, nil, &cfg)
	if err != nil {
		t.Fatal(err)
	}

	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/api/auth-info", mgr.HandleAuthInfo)
	mux.HandleFunc("/api/login", mgr.HandleLogin)
	mux.HandleFunc("/api/oidc/login", mgr.HandleOIDCLogin)
	mux.HandleFunc("/api/oidc/callback", mgr.HandleOIDCCallback)
	mux.HandleFunc("/api/me", mgr.JWTMiddleware(ok))
	mux.HandleFunc("/api/users", mgr.AdminMiddleware(ok))

	return &integrationEnv{server: server, store: store, auth: mgr}
}

type signInResult struct {
	token       string
	err         string
	callbackURL string
	client      *http.Client
}

func (e *integrationEnv) newBrowser(t *testing.T) *http.Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (e *integrationEnv) signIn(t *testing.T, username string, claims map[string]interface{}) signInResult {
	t.Helper()
	return e.signInWith(t, e.newBrowser(t), username, claims, nil)
}

func (e *integrationEnv) signInWith(t *testing.T, browser *http.Client, username string, claims map[string]interface{}, tamper func(*url.URL)) signInResult {
	t.Helper()

	resp := mustDo(t, browser, http.MethodGet, e.server.URL+"/api/oidc/login", nil)
	authorizeURL := redirectTarget(t, resp)
	if strings.HasPrefix(authorizeURL, "/#") {
		return parseFragment(t, authorizeURL, "", browser)
	}
	if q := mustParseURL(t, authorizeURL).Query(); q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
		t.Fatalf("authorise request is missing PKCE or nonce: %s", authorizeURL)
	}

	resp = mustDo(t, browser, http.MethodGet, authorizeURL, nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `name="username"`) {
		t.Fatalf("provider login page not served: %d %s", resp.StatusCode, body)
	}

	claimsJSON, _ := json.Marshal(claims)
	form := url.Values{"username": {username}, "claims": {string(claimsJSON)}}
	resp = mustDo(t, browser, http.MethodPost, authorizeURL, form)
	callback := mustParseURL(t, redirectTarget(t, resp))
	if tamper != nil {
		tamper(callback)
	}

	resp = mustDo(t, browser, http.MethodGet, callback.String(), nil)
	return parseFragment(t, redirectTarget(t, resp), callback.String(), browser)
}

func parseFragment(t *testing.T, location, callbackURL string, browser *http.Client) signInResult {
	t.Helper()
	if !strings.HasPrefix(location, "/#") {
		t.Fatalf("expected redirect to the admin UI, got %q", location)
	}
	frag, err := url.ParseQuery(strings.TrimPrefix(location, "/#"))
	if err != nil {
		t.Fatalf("bad fragment %q: %v", location, err)
	}
	return signInResult{token: frag.Get("oidc_token"), err: frag.Get("oidc_error"), callbackURL: callbackURL, client: browser}
}

func mustDo(t *testing.T, client *http.Client, method, target string, form url.Values) *http.Response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func redirectTarget(t *testing.T, resp *http.Response) string {
	t.Helper()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: want redirect, got %d: %s", resp.Request.Method, resp.Request.URL, resp.StatusCode, body)
	}
	return resp.Header.Get("Location")
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *integrationEnv) status(t *testing.T, path, token string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func defaultGroups() auth.OIDCConfig {
	return auth.OIDCConfig{
		AdminGroups:   []string{"bootimus-admins"},
		AllowedGroups: []string{"bootimus-operators"},
		DisplayName:   "Mock IdP",
	}
}

func TestOIDCIntegrationAuthInfo(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	resp, err := http.Get(env.server.URL + "/api/auth-info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	for _, b := range info.Data {
		if b["id"] == "oidc" && b["type"] == "redirect" && b["name"] == "Mock IdP" && b["url"] == "/api/oidc/login" {
			return
		}
	}
	t.Fatalf("OIDC backend not advertised: %+v", info.Data)
}

func TestOIDCIntegrationAdminSignIn(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	res := env.signIn(t, "alice", map[string]interface{}{
		"preferred_username": "alice",
		"groups":             []string{"staff", "bootimus-admins"},
	})
	if res.token == "" {
		t.Fatalf("sign-in failed: %q", res.err)
	}

	user, err := env.store.GetUser("alice")
	if err != nil {
		t.Fatalf("user not stored: %v", err)
	}
	if user.Source != models.UserSourceOIDC || !user.IsAdmin || !user.Enabled || user.LastLogin == nil {
		t.Fatalf("unexpected stored user: %+v", user)
	}
	if code := env.status(t, "/api/users", res.token); code != http.StatusOK {
		t.Fatalf("admin endpoint: want 200, got %d", code)
	}
}

func TestOIDCIntegrationRegularUser(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	res := env.signIn(t, "bob", map[string]interface{}{
		"preferred_username": "bob",
		"groups":             []string{"bootimus-operators"},
	})
	if res.token == "" {
		t.Fatalf("sign-in failed: %q", res.err)
	}
	if code := env.status(t, "/api/me", res.token); code != http.StatusOK {
		t.Fatalf("authenticated endpoint: want 200, got %d", code)
	}
	if code := env.status(t, "/api/users", res.token); code != http.StatusForbidden {
		t.Fatalf("admin endpoint: want 403, got %d", code)
	}
}

func TestOIDCIntegrationOutsiderRefused(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	res := env.signIn(t, "mallory", map[string]interface{}{
		"preferred_username": "mallory",
		"groups":             []string{"contractors"},
	})
	if res.token != "" || !strings.Contains(res.err, "not permitted") {
		t.Fatalf("want refusal, got %+v", res)
	}
	if _, err := env.store.GetUser("mallory"); err == nil {
		t.Fatal("refused user was created")
	}
}

func TestOIDCIntegrationNoGroupClaim(t *testing.T) {
	env := newIntegrationEnv(t, auth.OIDCConfig{AdminGroups: []string{"bootimus-admins"}})
	res := env.signIn(t, "carol", map[string]interface{}{"preferred_username": "carol"})
	if res.token == "" {
		t.Fatalf("sign-in without allowed groups configured should succeed: %q", res.err)
	}
	if code := env.status(t, "/api/users", res.token); code != http.StatusForbidden {
		t.Fatalf("user without groups must not be admin, got %d", code)
	}
}

func TestOIDCIntegrationGroupChangesApplyAtNextSignIn(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	first := env.signIn(t, "dave", map[string]interface{}{
		"preferred_username": "dave",
		"groups":             []string{"bootimus-admins"},
	})
	if first.token == "" {
		t.Fatalf("first sign-in failed: %q", first.err)
	}

	second := env.signIn(t, "dave", map[string]interface{}{
		"preferred_username": "dave",
		"groups":             []string{"bootimus-operators"},
	})
	if second.token == "" {
		t.Fatalf("second sign-in failed: %q", second.err)
	}
	if code := env.status(t, "/api/users", first.token); code != http.StatusForbidden {
		t.Fatalf("token issued before demotion: want 403, got %d", code)
	}

	third := env.signIn(t, "dave", map[string]interface{}{
		"preferred_username": "dave",
		"groups":             []string{"bootimus-admins"},
	})
	if code := env.status(t, "/api/users", third.token); code != http.StatusOK {
		t.Fatalf("re-promoted user: want 200, got %d", code)
	}
}

func TestOIDCIntegrationDisabledUser(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	claims := map[string]interface{}{"preferred_username": "erin", "groups": []string{"bootimus-admins"}}
	res := env.signIn(t, "erin", claims)
	if res.token == "" {
		t.Fatalf("sign-in failed: %q", res.err)
	}

	user, _ := env.store.GetUser("erin")
	user.Enabled = false
	if err := env.store.UpdateUser("erin", user); err != nil {
		t.Fatal(err)
	}

	if code := env.status(t, "/api/me", res.token); code != http.StatusUnauthorized {
		t.Fatalf("token of disabled user: want 401, got %d", code)
	}
	again := env.signIn(t, "erin", claims)
	if again.token != "" || !strings.Contains(again.err, "disabled") {
		t.Fatalf("disabled user signed in again: %+v", again)
	}
}

func TestOIDCIntegrationCannotTakeOverLocalAccount(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	res := env.signIn(t, "admin", map[string]interface{}{
		"preferred_username": "admin",
		"groups":             []string{"bootimus-admins"},
	})
	if res.token != "" {
		t.Fatal("OIDC sign-in took over the local admin account")
	}
	admin, err := env.store.GetUser("admin")
	if err != nil || admin.AuthSource() != models.UserSourceLocal {
		t.Fatalf("local admin changed: %+v (%v)", admin, err)
	}
}

func TestOIDCIntegrationLocalLoginRefusedForOIDCUser(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	if res := env.signIn(t, "frank", map[string]interface{}{"preferred_username": "frank", "groups": []string{"bootimus-admins"}}); res.token == "" {
		t.Fatalf("sign-in failed: %q", res.err)
	}
	user, _ := env.store.GetUser("frank")
	if err := user.SetPassword("guessable"); err != nil {
		t.Fatal(err)
	}
	if err := env.store.UpdateUser("frank", user); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(env.server.URL+"/api/login", "application/json", strings.NewReader(`{"username":"frank","password":"guessable"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("local password login for OIDC user: want 401, got %d", resp.StatusCode)
	}
}

func TestOIDCIntegrationEmailUsernameClaim(t *testing.T) {
	cfg := defaultGroups()
	cfg.UsernameClaim = "email"
	env := newIntegrationEnv(t, cfg)
	res := env.signIn(t, "grace", map[string]interface{}{
		"email":  "Grace@Example.com",
		"groups": []string{"bootimus-operators"},
	})
	if res.token == "" {
		t.Fatalf("sign-in failed: %q", res.err)
	}
	if _, err := env.store.GetUser("grace@example.com"); err != nil {
		t.Fatalf("user not stored under email: %v", err)
	}
}

func TestOIDCIntegrationCallbackTampering(t *testing.T) {
	env := newIntegrationEnv(t, defaultGroups())
	claims := map[string]interface{}{"preferred_username": "heidi", "groups": []string{"bootimus-admins"}}

	forged := env.signInWith(t, env.newBrowser(t), "heidi", claims, func(u *url.URL) {
		q := u.Query()
		q.Set("state", "forged")
		u.RawQuery = q.Encode()
	})
	if forged.token != "" || forged.err == "" {
		t.Fatalf("forged state accepted: %+v", forged)
	}

	ok := env.signIn(t, "heidi", claims)
	if ok.token == "" {
		t.Fatalf("sign-in failed: %q", ok.err)
	}
	replay := parseFragment(t, redirectTarget(t, mustDo(t, ok.client, http.MethodGet, ok.callbackURL, nil)), ok.callbackURL, ok.client)
	if replay.token != "" || replay.err == "" {
		t.Fatalf("replayed callback accepted: %+v", replay)
	}

	stolen := parseFragment(t, redirectTarget(t, mustDo(t, env.newBrowser(t), http.MethodGet, ok.callbackURL, nil)), ok.callbackURL, nil)
	if stolen.token != "" || stolen.err == "" {
		t.Fatalf("callback from another browser accepted: %+v", stolen)
	}
}

func TestOIDCIntegrationUnreachableProvider(t *testing.T) {
	cfg := defaultGroups()
	cfg.Issuer = "http://127.0.0.1:1/unreachable"
	env := newIntegrationEnv(t, cfg)
	res := env.signIn(t, "ivan", nil)
	if res.token != "" || !strings.Contains(res.err, "identity provider") {
		t.Fatalf("want provider error, got %+v", res)
	}
}
