package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AuthConfig struct {
	Secret   []byte
	Username string
	Password string
	Demo     bool
}
type Principal struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}
type principalKey struct{}
type claims struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	Expires int64  `json:"exp"`
	Issued  int64  `json:"iat"`
	Issuer  string `json:"iss"`
}

func actor(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalKey{}).(Principal); ok {
		return p
	}
	return Principal{Username: "system", Role: "admin"}
}
func validRole(role string) bool { return role == "admin" || role == "maintainer" || role == "viewer" }
func signJWT(c AuthConfig, p Principal, now time.Time) (string, error) {
	if len(c.Secret) < 32 || !validRole(p.Role) {
		return "", errors.New("invalid authentication configuration")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	b, e := json.Marshal(claims{p.Username, p.Role, now.Add(time.Hour).Unix(), now.Unix(), "sentinel-ci"})
	if e != nil {
		return "", e
	}
	body := header + "." + base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, c.Secret)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func verifyJWT(c AuthConfig, token string, now time.Time) (Principal, error) {
	invalid := errors.New("invalid or expired token")
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(c.Secret) < 32 {
		return Principal{}, invalid
	}
	h, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return Principal{}, invalid
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(h, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return Principal{}, invalid
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		return Principal{}, invalid
	}
	mac := hmac.New(sha256.New, c.Secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Principal{}, invalid
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return Principal{}, invalid
	}
	var v claims
	if json.Unmarshal(b, &v) != nil || v.Issuer != "sentinel-ci" || v.Subject == "" || !validRole(v.Role) || v.Expires <= now.Unix() || v.Issued > now.Add(time.Minute).Unix() || v.Expires-v.Issued > 3600 {
		return Principal{}, invalid
	}
	return Principal{v.Subject, v.Role}, nil
}
func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func equalSecret(a, b string) bool {
	x := sha256.Sum256([]byte(a))
	y := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}
func allowed(role, method, path string) bool {
	if role == "admin" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/audit-logs") || strings.HasPrefix(path, "/api/v1/auth/keys") {
		return false
	}
	if method == http.MethodGet {
		return role == "viewer" || role == "maintainer"
	}
	return role == "maintainer"
}
func (a *App) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.testMode || r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/webhooks/github" {
			next.ServeHTTP(w, r)
			return
		}
		if a.auth == nil {
			fail(w, 503, errors.New("authentication unavailable"))
			return
		}
		var p Principal
		var err error
		token := r.Header.Get("X-Sentinel-Token")
		if token != "" {
			err = a.db.QueryRow(r.Context(), "SELECT name,role FROM api_keys WHERE token_hash=$1 AND revoked_at IS NULL", hashToken(token)).Scan(&p.Username, &p.Role)
		} else {
			bearer := r.Header.Get("Authorization")
			if strings.HasPrefix(bearer, "Bearer ") {
				token = strings.TrimPrefix(bearer, "Bearer ")
			} else if strings.HasPrefix(r.URL.Path, "/ws/") {
				token = r.URL.Query().Get("token")
			}
			p, err = verifyJWT(*a.auth, token, time.Now())
		}
		if err != nil || !validRole(p.Role) {
			fail(w, 401, errors.New("authentication required"))
			return
		}
		if !allowed(p.Role, r.Method, r.URL.Path) {
			fail(w, 403, errors.New("role does not permit this operation"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		fail(w, 503, errors.New("authentication unavailable"))
		return
	}
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Demo     bool   `json:"demo"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &b) {
		return
	}
	p := Principal{Username: a.auth.Username, Role: "admin"}
	if b.Demo {
		if !a.auth.Demo || b.Role != "admin" && b.Role != "viewer" {
			fail(w, 403, errors.New("demo sessions unavailable for requested role"))
			return
		}
		p = Principal{Username: "demo-" + b.Role, Role: b.Role}
	} else if b.Username != a.auth.Username || !equalSecret(b.Password, a.auth.Password) {
		fail(w, 401, errors.New("invalid credentials"))
		return
	}
	token, e := signJWT(*a.auth, p, time.Now())
	if e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 200, map[string]any{"token": token, "username": p.Username, "role": p.Role, "expires_at": time.Now().Add(time.Hour).UTC()})
}
func (a *App) apiKeys(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		Name   string `json:"name"`
		Role   string `json:"role"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Action == "revoke" {
		if !validID(b.ID) {
			fail(w, 400, errors.New("valid key id required"))
			return
		}
		tx, e := a.db.Begin(r.Context())
		if e != nil {
			fail(w, 503, e)
			return
		}
		defer tx.Rollback(r.Context())
		tag, e := tx.Exec(r.Context(), "UPDATE api_keys SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL", b.ID)
		if e != nil {
			fail(w, 500, e)
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 404, errors.New("active API key not found"))
			return
		}
		if e = auditTx(r.Context(), tx, "api_key.revoke", "api_key", b.ID, map[string]any{}); e != nil {
			fail(w, 500, e)
			return
		}
		if e = tx.Commit(r.Context()); e != nil {
			fail(w, 503, e)
			return
		}
		write(w, 200, map[string]any{"id": b.ID, "revoked": true})
		return
	}
	if b.Action != "" && b.Action != "create" || strings.TrimSpace(b.Name) == "" || !validRole(b.Role) {
		fail(w, 400, errors.New("name and valid role required"))
		return
	}
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		fail(w, 500, e)
		return
	}
	token := "sentinel_sk_" + base64.RawURLEncoding.EncodeToString(raw)
	id := uuid()
	tx, e := a.db.Begin(r.Context())
	if e != nil {
		fail(w, 503, e)
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "INSERT INTO api_keys(id,name,token_hash,role) VALUES($1,$2,$3,$4)", id, b.Name, hashToken(token), b.Role); e != nil {
		fail(w, 500, e)
		return
	}
	if e = auditTx(r.Context(), tx, "api_key.create", "api_key", id, map[string]any{"name": b.Name, "role": b.Role}); e != nil {
		fail(w, 500, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 201, map[string]any{"id": id, "name": b.Name, "role": b.Role, "token": token})
}
func verifySignature(secret string, body []byte, header string) bool {
	if secret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	sig, e := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if e != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}
func (a *App) webhook(w http.ResponseWriter, r *http.Request) {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		fail(w, 400, e)
		return
	}
	var b struct {
		After      string `json:"after"`
		Ref        string `json:"ref"`
		Repository struct {
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		HeadCommit struct {
			Message string `json:"message"`
		} `json:"head_commit"`
	}
	if json.Unmarshal(body, &b) != nil || !strings.HasPrefix(b.Ref, "refs/heads/") {
		fail(w, 400, errors.New("valid push payload required"))
		return
	}
	var id, secret string
	if e = a.db.QueryRow(r.Context(), "SELECT id::text,webhook_secret FROM repositories WHERE repo_url=$1 OR repo_url=$2", b.Repository.CloneURL, b.Repository.HTMLURL).Scan(&id, &secret); e != nil {
		fail(w, 401, errors.New("webhook signature invalid"))
		return
	}
	if !verifySignature(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		fail(w, 401, errors.New("webhook signature invalid"))
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		fail(w, 400, errors.New("only push events supported"))
		return
	}
	if !validSHA(b.After) {
		fail(w, 400, errors.New("invalid commit SHA"))
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	if len(delivery) < 1 || len(delivery) > 200 {
		fail(w, 400, errors.New("delivery id required"))
		return
	}
	ctx := context.WithValue(r.Context(), principalKey{}, Principal{"github-webhook", "maintainer"})
	j := Job{PipelineID: uuid(), RepoURL: b.Repository.CloneURL, CommitSHA: b.After, Branch: strings.TrimPrefix(b.Ref, "refs/heads/"), CommitMessage: b.HeadCommit.Message, IdempotencyKey: "webhook:" + delivery, Steps: []Step{{Name: "Checkout", Stage: "checkout", Command: "find . -maxdepth 2 -type f | sort"}, {Name: "Unit Test Suite", Stage: "test", Command: "if [ -f go.mod ]; then go test ./...; else python3 -m unittest discover -v; fi", DependsOn: []string{"Checkout"}}}}
	if e = a.createJob(ctx, id, &j); e != nil {
		fail(w, 503, e)
		return
	}
	write(w, 202, map[string]string{"id": j.PipelineID, "pipeline_id": j.PipelineID, "status": "PENDING"})
}
func validSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	_, e := hex.DecodeString(s)
	if len(s)%2 == 1 {
		_, e = hex.DecodeString("0" + s)
	}
	return e == nil
}
func authConfigFromEnv() (AuthConfig, error) {
	c := AuthConfig{Secret: []byte(env("JWT_SECRET", "")), Username: env("ADMIN_USERNAME", "admin"), Password: env("ADMIN_PASSWORD", ""), Demo: env("SENTINEL_DEMO_MODE", "false") == "true"}
	return c, validateAuthConfig(c, env("INTERNAL_TOKEN", ""))
}

func validateAuthConfig(c AuthConfig, internalToken string) error {
	if len(c.Secret) < 32 || len(c.Password) < 12 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes and ADMIN_PASSWORD at least 12 characters")
	}
	if !c.Demo {
		if len(internalToken) < 32 {
			return errors.New("production INTERNAL_TOKEN must be at least 32 bytes")
		}
		for _, secret := range []string{string(c.Secret), c.Password, internalToken} {
			if strings.HasPrefix(secret, "sentinel-local-") {
				return errors.New("local demonstration secrets cannot be used when demo mode is disabled")
			}
		}
	}
	return nil
}
