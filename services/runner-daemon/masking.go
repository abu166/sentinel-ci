package main

import (
	"math"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const redacted = "[REDACTED]"

var (
	sensitiveKey   = regexp.MustCompile(`(?i)(?:["']?)(?:[a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|credential)[a-z0-9_.-]*|authorization)(?:["']?)\s*[:=]\s*`)
	bearerToken    = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[A-Za-z0-9_+/=.-]+`)
	jwtToken       = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	prefixedToken  = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{12,}|github_pat_[A-Za-z0-9_]{12,}|sk-(?:proj-)?[A-Za-z0-9_-]{12,}|xox[baprs]-[A-Za-z0-9-]{10,}|(?:AKIA|ASIA)[A-Z0-9]{16}|AIza[0-9A-Za-z_-]{20,})\b`)
	urlCredential  = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s/@:]+:[^\s/@]+@`)
	entropyToken   = regexp.MustCompile(`[A-Za-z0-9_+/=-]{24,}`)
	manifestDigest = regexp.MustCompile(`^[a-fA-F0-9]{64}\s+\*?[^\s]+$`)
	plainHex       = regexp.MustCompile(`^[a-fA-F0-9]+$`)
	privateBegin   = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----`)
	privateEnd     = regexp.MustCompile(`-----END (?:[A-Z0-9]+ )?PRIVATE KEY-----`)
)

type maskingState struct {
	privateKey  bool
	quote       byte
	nextValue   bool
	block       bool
	blockIndent int
	triple      string
}

// SecretMasker is stateful per stream so multiline keys and quoted credentials do
// not expose fragments. It never records original matching values in telemetry.
type SecretMasker struct {
	mu    sync.Mutex
	known []string
	state map[string]*maskingState
	count int
}

func NewSecretMasker(known []string) *SecretMasker {
	values := make([]string, 0, len(known))
	seen := map[string]bool{}
	for _, v := range known {
		if len(v) >= 4 && !seen[v] {
			values = append(values, v)
			seen[v] = true
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return &SecretMasker{known: values, state: map[string]*maskingState{}}
}
func maskerFromEnvironment() *SecretMasker {
	var secrets []string
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if sensitiveKey.MatchString(name + "=") {
			secrets = append(secrets, value)
		}
		if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
			if password, ok := parsed.User.Password(); ok {
				secrets = append(secrets, password)
			}
		}
	}
	return NewSecretMasker(secrets)
}
func entropy(value string) float64 {
	if len(value) == 0 {
		return 0
	}
	counts := map[rune]int{}
	length := 0
	for _, r := range value {
		counts[r]++
		length++
	}
	score := 0.0
	for _, n := range counts {
		p := float64(n) / float64(length)
		score -= p * math.Log2(p)
	}
	return score
}
func looksSecret(value string) bool {
	if value == redacted || strings.HasPrefix(value, "/") {
		return false
	}
	if plainHex.MatchString(value) {
		return len(value) >= 48 && entropy(value) >= 3.5
	}
	lower, upper, digit, symbol := false, false, false, false
	for _, r := range value {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			symbol = true
		}
	}
	return lower && upper && digit && (symbol || len(value) >= 32) && entropy(value) >= 4.3
}
func closingQuote(s string, quote byte) int {
	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == quote {
			return i
		}
	}
	return -1
}
func (m *SecretMasker) Filter(stream, line string) (string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		m.state = map[string]*maskingState{}
	}
	state := m.state[stream]
	if state == nil {
		state = &maskingState{}
		m.state[stream] = state
	}
	count := 0
	if state.block {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if strings.TrimSpace(line) == "" || indent > state.blockIndent {
			m.count++
			return redacted, 1
		}
		state.block = false
	}
	if state.triple != "" {
		end := strings.Index(line, state.triple)
		if end < 0 {
			m.count++
			return redacted, 1
		}
		line = redacted + line[end+3:]
		state.triple = ""
		count++
	}
	if state.privateKey {
		if privateEnd.MatchString(line) {
			state.privateKey = false
		}
		m.count++
		return redacted, 1
	}
	if privateBegin.MatchString(line) {
		state.privateKey = !privateEnd.MatchString(line)
		m.count++
		return redacted, 1
	}
	if state.quote != 0 {
		end := closingQuote(line, state.quote)
		if end < 0 {
			m.count++
			return redacted, 1
		}
		line = redacted + line[end+1:]
		state.quote = 0
		count++
	}
	if state.nextValue && strings.TrimSpace(line) != "" {
		state.nextValue = false
		m.count++
		return redacted, 1
	}
	for _, secret := range m.known {
		if strings.Contains(line, secret) {
			count += strings.Count(line, secret)
			line = strings.ReplaceAll(line, secret, redacted)
		}
	}
	for offset, replacements := 0, 0; offset < len(line); replacements++ {
		if replacements >= 64 {
			line = line[:offset] + redacted
			count++
			break
		}
		loc := sensitiveKey.FindStringIndex(line[offset:])
		if loc == nil {
			break
		}
		end := offset + loc[1]
		if end >= len(line) {
			line += redacted
			state.nextValue = true
			count++
			break
		}
		if strings.HasPrefix(line[end:], redacted) {
			offset = end + len(redacted)
			continue
		}
		value := line[end:]
		if value == "|" || value == ">" || value == "|-" || value == ">-" || value == "|+" || value == ">+" {
			state.block = true
			state.blockIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			line = line[:end] + redacted
			count++
			break
		}
		if strings.HasPrefix(value, strings.Repeat("\"", 3)) || strings.HasPrefix(value, strings.Repeat("'", 3)) {
			delimiter := value[:3]
			tail := strings.Index(value[3:], delimiter)
			if tail < 0 {
				state.triple = delimiter
				line = line[:end] + redacted
			} else {
				line = line[:end] + redacted + value[tail+6:]
			}
			count++
			break
		}
		quote := line[end]
		if quote == '"' || quote == '\'' {
			close := closingQuote(line[end+1:], quote)
			if close >= 0 {
				tail := end + 1 + close
				line = line[:end+1] + redacted + line[tail:]
				offset = end + 1 + len(redacted) + 1
			} else {
				line = line[:end+1] + redacted
				state.quote = quote
				offset = len(line)
			}
		} else {
			line = line[:end] + redacted
			offset = len(line)
		}
		count++
	}
	for _, pattern := range []*regexp.Regexp{bearerToken, jwtToken, prefixedToken} {
		line = pattern.ReplaceAllStringFunc(line, func(string) string { count++; return redacted })
	}
	line = urlCredential.ReplaceAllStringFunc(line, func(value string) string {
		count++
		scheme, _, _ := strings.Cut(value, "://")
		return scheme + "://" + redacted + "@"
	})
	if !manifestDigest.MatchString(strings.TrimSpace(line)) {
		line = entropyToken.ReplaceAllStringFunc(line, func(value string) string {
			if looksSecret(value) {
				count++
				return redacted
			}
			return value
		})
	}
	m.count += count
	return line, count
}
func (m *SecretMasker) Redactions() int { m.mu.Lock(); defer m.mu.Unlock(); return m.count }
