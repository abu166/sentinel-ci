package main

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestMaskingSensitiveFormats(t *testing.T) {
	cases := []struct{ name, line, secret string }{
		{"password", "database password=correct-horse-battery-staple", "correct-horse-battery-staple"},
		{"json", `{"password":"words with spaces", "status":"ok"}`, "words with spaces"},
		{"escaped_quote", `{"api_key":"abc\"defsecret", "status":"ok"}`, `abc\"defsecret`},
		{"environment", "AWS_SECRET_ACCESS_KEY=abcdefghijklmnop", "abcdefghijklmnop"},
		{"bearer", "Authorization: Bearer short-opaque-value", "short-opaque-value"},
		{"basic", "Authorization: Basic dXNlcjpwYXNzd29yZA==", "dXNlcjpwYXNzd29yZA=="},
		{"jwt", "value eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZW50aW5lbCJ9.dGVzdHNpZ25hdHVyZQ end", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJzZW50aW5lbCJ9.dGVzdHNpZ25hdHVyZQ"},
		{"github", "token is ghp_AbCdEf0123456789XYZabcdefgh", "ghp_AbCdEf0123456789XYZabcdefgh"},
		{"github_fine", "github_pat_AbCdEf0123456789abcdefghijkl", "github_pat_AbCdEf0123456789abcdefghijkl"},
		{"openai", "sk-proj-AbCdEf0123456789abcdefghijkl", "sk-proj-AbCdEf0123456789abcdefghijkl"},
		{"slack", "xoxb-1234567890-abcdefghijk", "xoxb-1234567890-abcdefghijk"},
		{"aws", "access AKIA0123456789ABCDEF", "AKIA0123456789ABCDEF"},
		{"google", "key AIzaAbCdEf0123456789abcdefghijkl", "AIzaAbCdEf0123456789abcdefghijkl"},
		{"url", "connecting postgres://sentinel:secret-password@postgres:5432/db", "secret-password"},
		{"entropy", "opaque 9fQaA2pMzU7kRt4xNv8cLy5wHs1eJg6d", "9fQaA2pMzU7kRt4xNv8cLy5wHs1eJg6d"},
		{"hex_entropy", "opaque 9fca82d731b604e2a9817cf53a64d0bef924670c28d158bf01372ed94a67b5c0", "9fca82d731b604e2a9817cf53a64d0bef924670c28d158bf01372ed94a67b5c0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewSecretMasker(nil)
			got, n := m.Filter("stdout", tc.line)
			if strings.Contains(got, tc.secret) || !strings.Contains(got, redacted) || n < 1 {
				t.Fatal("sensitive value was not redacted")
			}
		})
	}
}
func TestMaskingPreservesJSONStructure(t *testing.T) {
	got, _ := NewSecretMasker(nil).Filter("stdout", `{"password":"hello, world", "api_key":"abcdef", "status":"ok"}`)
	if got != `{"password":"[REDACTED]", "api_key":"[REDACTED]", "status":"ok"}` {
		t.Fatal(got)
	}
}
func TestMaskingPreservesOperationalIdentifiers(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{"--- FAIL: TestStockBoundary (0.00s)", "/workspace/pkg/inventory/stock.go:84 +0x123", "trace_id=9fca82d731b604e2a9817cf53a64d0be", "commit_sha=9fca82d731b604e2a9817cf53a64d0bef924670c", "9fca82d731b604e2a9817cf53a64d0bef924670c28d158bf01372ed94a67b5c0  inventory.py", "normal worker output 42 passed"} {
		if got, n := m.Filter("stdout", line); got != line || n != 0 {
			t.Fatal("operational identifier was over-redacted", got)
		}
	}
}
func TestMaskingKnownEnvironmentSecrets(t *testing.T) {
	t.Setenv("SENTINEL_TEST_TOKEN", "small-configured-value")
	m := maskerFromEnvironment()
	got, _ := m.Filter("stderr", "unlabeled small-configured-value")
	if strings.Contains(got, "small-configured-value") {
		t.Fatal("configured secret leaked")
	}
}
func TestMaskingLongestExactMatchFirst(t *testing.T) {
	m := NewSecretMasker([]string{"abcd", "abcdefghi", "abcd"})
	got, _ := m.Filter("stdout", "abcdefghi")
	if got != redacted {
		t.Fatal(got)
	}
}
func TestMaskingMultilinePrivateKey(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{"-----BEGIN RSA PRIVATE KEY-----", "abcdefghijklmnop", "-----END RSA PRIVATE KEY-----"} {
		got, n := m.Filter("stdout", line)
		if got != redacted || n < 1 {
			t.Fatal(got, n)
		}
	}
	got, _ := m.Filter("stdout", "safe log")
	if got != "safe log" {
		t.Fatal(got)
	}
}
func TestMaskingQuotedContinuation(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{`password="first-fragment`, "second-fragment", `third-fragment" after`} {
		got, _ := m.Filter("stdout", line)
		if strings.Contains(got, "fragment") {
			t.Fatal("multiline credential leaked")
		}
	}
	got, _ := m.Filter("stdout", "safe")
	if got != "safe" {
		t.Fatal(got)
	}
}
func TestMaskingNextLineValue(t *testing.T) {
	m := NewSecretMasker(nil)
	m.Filter("stdout", "password=")
	got, _ := m.Filter("stdout", "secret-on-next-line")
	if got != redacted {
		t.Fatal(got)
	}
}
func TestMaskingStateIsolatedPerStream(t *testing.T) {
	m := NewSecretMasker(nil)
	m.Filter("stdout", "-----BEGIN PRIVATE KEY-----")
	got, _ := m.Filter("stderr", "Traceback (most recent call last):")
	if got != "Traceback (most recent call last):" {
		t.Fatal(got)
	}
}
func TestMaskingConcurrentStreamsRaceSafe(t *testing.T) {
	m := NewSecretMasker(nil)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, _ := m.Filter("stdout", "token=never-expose-me")
				if strings.Contains(got, "never-expose-me") {
					t.Error("secret leaked")
				}
			}
		}()
	}
	wg.Wait()
	if m.Redactions() < 3200 {
		t.Fatal(m.Redactions())
	}
}
func TestMaskingEntropyThreshold(t *testing.T) {
	if entropy("aaaaaaaaaaaa") != 0 || entropy("0123456789abcdef") != 4 {
		t.Fatal("entropy formula")
	}
	if looksSecret("ordinary-function-name") || looksSecret("9fca82d731b604e2a9817cf53a64d0be") {
		t.Fatal("ordinary identifiers classified as secrets")
	}
}
func TestMaskingSplitWritesBeforeLineBoundary(t *testing.T) {
	var lines []string
	m := NewSecretMasker(nil)
	code, _, err := runCommand(context.Background(), t.TempDir(), `printf 'pass'; printf 'word=split-secret\n'`, func(stream, line string) { safe, _ := m.Filter(stream, line); lines = append(lines, safe) })
	if err != nil || code != 0 || len(lines) != 1 || strings.Contains(lines[0], "split-secret") {
		t.Fatal("split write leaked")
	}
}
func TestMaskingEmptyAndAlreadyMasked(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{"", redacted, "token=" + redacted} {
		got, _ := m.Filter("stdout", line)
		if got != line {
			t.Fatal(got)
		}
	}
}
func TestMaskingManyKeysIsBoundedFailClosed(t *testing.T) {
	m := NewSecretMasker(nil)
	line := "{" + strings.Repeat(`"password":"sensitive",`, 1000) + `"last":"safe"}`
	got, _ := m.Filter("stdout", line)
	if strings.Contains(got, "sensitive") || len(got) > 5000 {
		t.Fatal("unbounded or leaking redaction")
	}
}

func TestMaskingYAMLBlockEndsAtDedent(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{"  password: |-", "    short fragment", "    another fragment"} {
		value, _ := m.Filter("stdout", line)
		if strings.Contains(value, "fragment") || !strings.Contains(value, redacted) {
			t.Fatal(value)
		}
	}
	if value, _ := m.Filter("stdout", "  status: ready"); value != "  status: ready" {
		t.Fatal(value)
	}
}
func TestMaskingTripleQuotedCredential(t *testing.T) {
	m := NewSecretMasker(nil)
	for _, line := range []string{"token = \"\"\"", "short fragment", "last fragment\"\"\""} {
		value, _ := m.Filter("stdout", line)
		if strings.Contains(value, "fragment") || !strings.Contains(value, redacted) {
			t.Fatal(value)
		}
	}
	if value, _ := m.Filter("stdout", "complete"); value != "complete" {
		t.Fatal(value)
	}
}
