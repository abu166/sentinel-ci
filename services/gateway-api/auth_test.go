package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func testAuth() *AuthConfig {
	return &AuthConfig{Secret: []byte(strings.Repeat("s", 32)), Username: "admin", Password: "strong-secret-password", Demo: true}
}
func TestJWTValidRoundTrip(t *testing.T) {
	c := testAuth()
	now := time.Now()
	token, e := signJWT(*c, Principal{"engineer", "maintainer"}, now)
	if e != nil {
		t.Fatal(e)
	}
	p, e := verifyJWT(*c, token, now)
	if e != nil || p.Username != "engineer" || p.Role != "maintainer" {
		t.Fatalf("%+v %v", p, e)
	}
}
func TestJWTRejectTampering(t *testing.T) {
	c := testAuth()
	now := time.Now()
	token, _ := signJWT(*c, Principal{"engineer", "admin"}, now)
	for _, bad := range []string{"", token + "x", "x." + strings.Split(token, ".")[1] + ".x", strings.Replace(token, ".", "", 1)} {
		if _, e := verifyJWT(*c, bad, now); e == nil {
			t.Fatal("accepted invalid token")
		}
	}
}
func TestJWTRejectExpiryAndFuture(t *testing.T) {
	c := testAuth()
	now := time.Now()
	expired, _ := signJWT(*c, Principal{"admin", "admin"}, now.Add(-2*time.Hour))
	future, _ := signJWT(*c, Principal{"admin", "admin"}, now.Add(2*time.Hour))
	for _, token := range []string{expired, future} {
		if _, e := verifyJWT(*c, token, now); e == nil {
			t.Fatal("accepted invalid token time")
		}
	}
}
func TestJWTWrongKeyAndRole(t *testing.T) {
	c := testAuth()
	token, _ := signJWT(*c, Principal{"admin", "admin"}, time.Now())
	other := *c
	other.Secret = []byte(strings.Repeat("z", 32))
	if _, e := verifyJWT(other, token, time.Now()); e == nil {
		t.Fatal("wrong key accepted")
	}
	if _, e := signJWT(*c, Principal{"admin", "owner"}, time.Now()); e == nil {
		t.Fatal("unknown role accepted")
	}
}
func TestRoleAuthorization(t *testing.T) {
	cases := []struct {
		role, method, path string
		allow              bool
	}{{"viewer", "GET", "/api/v1/pipelines", true}, {"viewer", "POST", "/api/v1/pipelines/trigger", false}, {"viewer", "GET", "/api/v1/audit-logs", false}, {"maintainer", "PATCH", "/api/v1/repos/id", true}, {"maintainer", "POST", "/api/v1/auth/keys", false}, {"admin", "POST", "/api/v1/auth/keys", true}, {"unknown", "GET", "/api/v1/repos", false}}
	for _, c := range cases {
		if got := allowed(c.role, c.method, c.path); got != c.allow {
			t.Fatalf("%+v got%v", c, got)
		}
	}
}
func TestAuthenticationRequiresToken(t *testing.T) {
	a := &App{auth: testAuth()}
	w := httptest.NewRecorder()
	a.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("unauthenticated reached handler") })).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/repos", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestAuthenticationViewerCannotMutate(t *testing.T) {
	a := &App{auth: testAuth()}
	token, _ := signJWT(*a.auth, Principal{"reader", "viewer"}, time.Now())
	r := httptest.NewRequest("POST", "/api/v1/pipelines/trigger", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("viewer mutation reached handler") })).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestAuthenticationJWTContextAndWS(t *testing.T) {
	a := &App{auth: testAuth()}
	token, _ := signJWT(*a.auth, Principal{"reader", "viewer"}, time.Now())
	for _, path := range []string{"/api/v1/repos", "/ws/pipelines/id/logs?token=" + token} {
		r := httptest.NewRequest("GET", path, nil)
		if !strings.HasPrefix(path, "/ws/") {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		a.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if actor(r.Context()).Username != "reader" {
				t.Fatal("missing principal")
			}
			w.WriteHeader(204)
		})).ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
}
func TestAPIKeyAuthentication(t *testing.T) {
	a, db := mockApp(t)
	a.auth = testAuth()
	a.testMode = false
	token := "sentinel_sk_secret"
	db.ExpectQuery("SELECT name,role FROM api_keys").WithArgs(hashToken(token)).WillReturnRows(pgxmock.NewRows([]string{"name", "role"}).AddRow("build-agent", "maintainer"))
	r := httptest.NewRequest("GET", "/api/v1/repos", nil)
	r.Header.Set("X-Sentinel-Token", token)
	w := httptest.NewRecorder()
	a.authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if actor(r.Context()).Role != "maintainer" {
			t.Fatal("missing key principal")
		}
		w.WriteHeader(204)
	})).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	if e := db.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
func TestTokenHashDoesNotStoreSecret(t *testing.T) {
	a, b := hashToken("token-a"), hashToken("token-b")
	if len(a) != 64 || a == b || strings.Contains(a, "token") {
		t.Fatal(a, b)
	}
}
func TestLoginAdminCredentials(t *testing.T) {
	a := &App{auth: testAuth()}
	for _, c := range []struct {
		body   string
		status int
	}{{`{"username":"admin","password":"strong-secret-password"}`, 200}, {`{"username":"admin","password":"wrong"}`, 401}, {`{"username":"other","password":"strong-secret-password"}`, 401}, {`{`, 400}} {
		w := httptest.NewRecorder()
		a.login(w, httptest.NewRequest("POST", "/", strings.NewReader(c.body)))
		if w.Code != c.status {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if c.status == 200 {
			var v map[string]any
			json.Unmarshal(w.Body.Bytes(), &v)
			if v["role"] != "admin" || v["token"] == "" {
				t.Fatal(v)
			}
		}
	}
}
func TestDemoLoginExplicitlyEnabled(t *testing.T) {
	a := &App{auth: testAuth()}
	for _, role := range []string{"admin", "viewer", "maintainer"} {
		w := httptest.NewRecorder()
		a.login(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"demo":true,"role":"`+role+`"}`)))
		want := 200
		if role == "maintainer" {
			want = 403
		}
		if w.Code != want {
			t.Fatal(w.Code)
		}
	}
	a.auth.Demo = false
	w := httptest.NewRecorder()
	a.login(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"demo":true,"role":"admin"}`)))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestSecureAuthEnvironment(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("ADMIN_PASSWORD", "")
	if _, e := authConfigFromEnv(); e == nil {
		t.Fatal("empty config accepted")
	}
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("ADMIN_PASSWORD", "long-enough-password")
	t.Setenv("SENTINEL_DEMO_MODE", "false")
	t.Setenv("INTERNAL_TOKEN", strings.Repeat("i", 32))
	c, e := authConfigFromEnv()
	if e != nil || c.Demo {
		t.Fatal(c, e)
	}
}

func TestProductionRejectsKnownDemoSecrets(t *testing.T) {
	for _, tc := range []struct{ name, jwt, password, internal string }{
		{"jwt", "sentinel-local-jwt-secret-change-before-exposing-2026", "secure-admin-password", strings.Repeat("i", 32)},
		{"password", strings.Repeat("s", 32), "sentinel-local-admin-password", strings.Repeat("i", 32)},
		{"internal", strings.Repeat("s", 32), "secure-admin-password", "sentinel-local-internal-token-change-before-exposing"},
		{"missing internal", strings.Repeat("s", 32), "secure-admin-password", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := AuthConfig{Secret: []byte(tc.jwt), Password: tc.password}
			if validateAuthConfig(c, tc.internal) == nil {
				t.Fatal("production demonstration secret accepted")
			}
			c.Demo = true
			if err := validateAuthConfig(c, tc.internal); err != nil {
				t.Fatal("explicit demo configuration rejected", err)
			}
		})
	}
}
func TestWebhookHMACVerification(t *testing.T) {
	secret := "repository-webhook-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	signature := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !verifySignature(secret, body, signature) {
		t.Fatal("valid signature rejected")
	}
	for _, c := range []struct {
		secret string
		body   []byte
		sig    string
	}{{"", body, signature}, {secret, []byte("tampered"), signature}, {secret, body, "sha1=bad"}, {secret, body, "sha256=zz"}} {
		if verifySignature(c.secret, c.body, c.sig) {
			t.Fatal("invalid signature accepted")
		}
	}
}
func TestWebhookRejectsUnsignedPush(t *testing.T) {
	a, db := mockApp(t)
	id := uuid()
	db.ExpectQuery("SELECT id::text,webhook_secret").WithArgs("https://github.com/acme/repo", "https://github.com/acme/repo").WillReturnRows(pgxmock.NewRows([]string{"id", "secret"}).AddRow(id, "strong-webhook-secret"))
	w := httptest.NewRecorder()
	a.webhook(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"ref":"refs/heads/main","repository":{"clone_url":"https://github.com/acme/repo","html_url":"https://github.com/acme/repo"}}`)))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestWebhookRejectsMalformedAndOversized(t *testing.T) {
	a := &App{}
	for _, b := range []string{`{`, `{"ref":"refs/tags/v1"}`, strings.Repeat("x", (1<<20)+1)} {
		w := httptest.NewRecorder()
		a.webhook(w, httptest.NewRequest("POST", "/", strings.NewReader(b)))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
func TestCommitSHAValidation(t *testing.T) {
	for _, s := range []string{"a8f92d1", strings.Repeat("a", 40), "7f3a9c2"} {
		if !validSHA(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"demo", "abc", "not-hex-value", strings.Repeat("a", 41)} {
		if validSHA(s) {
			t.Fatal(s)
		}
	}
}
func TestAPIKeyCreationAndRevocationValidation(t *testing.T) {
	a := &App{}
	for _, body := range []string{`{}`, `{"name":"a","role":"root"}`, `{"action":"revoke","id":"bad"}`, `{"name":"a","role":"viewer","action":"erase"}`} {
		w := httptest.NewRecorder()
		a.apiKeys(w, httptest.NewRequest("POST", "/", bytes.NewBufferString(body)))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
func TestPrincipalContextDefaults(t *testing.T) {
	if actor(context.Background()).Username != "system" {
		t.Fatal("missing default")
	}
	p := Principal{"user", "viewer"}
	if actor(context.WithValue(context.Background(), principalKey{}, p)) != p {
		t.Fatal("principal lost")
	}
}
