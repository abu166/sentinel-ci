package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTraceRetentionIsBounded(t *testing.T) {
	var buffer bytes.Buffer
	appendTrace(&buffer, strings.Repeat("a", 2_100_000))
	if buffer.Len() != 2_000_000 {
		t.Fatalf("retained %d bytes", buffer.Len())
	}
	appendTrace(&buffer, "IndexError: failure location")
	if buffer.Len() != 2_000_000 || !strings.HasSuffix(buffer.String(), "IndexError: failure location\n") {
		t.Fatal("latest failure context was not retained")
	}
}

func TestHeartbeatValues(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name   string
		values []interface{}
		ok     bool
	}{
		{"healthy", []interface{}{now.Format(time.RFC3339), now.Add(-10 * time.Second).Format(time.RFC3339)}, true},
		{"missing", []interface{}{nil}, false},
		{"stale", []interface{}{now.Add(-20 * time.Second).Format(time.RFC3339)}, false},
		{"future", []interface{}{now.Add(time.Minute).Format(time.RFC3339)}, false},
		{"malformed", []interface{}{"invalid"}, false},
		{"empty", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkHeartbeatValues(tc.values, now) == nil; got != tc.ok {
				t.Fatalf("health=%v expected=%v", got, tc.ok)
			}
		})
	}
}

func TestRunCommandStreams(t *testing.T) {
	var mu sync.Mutex
	lines := map[string][]string{}
	code, memory, err := runCommand(context.Background(), t.TempDir(), "printf 'one\ntwo\n'; printf 'failure\n' >&2; exit 7", func(stream, line string) { mu.Lock(); defer mu.Unlock(); lines[stream] = append(lines[stream], line) })
	if err != nil || code != 7 || memory < 0 {
		t.Fatalf("%d %d %v", code, memory, err)
	}
	if len(lines["stdout"]) != 2 || len(lines["stderr"]) != 1 {
		t.Fatal(lines)
	}
}
func TestTimeoutKillsProcessGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, _, err := runCommand(ctx, t.TempDir(), "sleep 20 & wait", func(string, string) {})
	if code != 124 || err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("%d %v %v", code, err, time.Since(start))
	}
}
func TestWorkerExecutionStress(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _, err := runCommand(context.Background(), t.TempDir(), fmt.Sprintf("printf '%d\\n'", i), func(string, string) {})
			if code != 0 || err != nil {
				t.Errorf("%d %v", code, err)
			}
		}(i)
	}
	wg.Wait()
}
func TestSafeWrite(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../escape", "/tmp/escape", ".", "../../a"} {
		if safeWrite(root, path, "bad") == nil {
			t.Fatal(path)
		}
	}
	if err := safeWrite(root, "src/app.py", "print(1)"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "src/app.py"))
	if err != nil || string(b) != "print(1)" {
		t.Fatal(err)
	}
}
func TestCollectFiles(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"app.py", "test_app.py", "go.mod", ".git/secret.py", "node_modules/pkg.py", "image.png"} {
		if err := safeWrite(root, path, "content"); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "link.py")); err != nil {
		t.Fatal(err)
	}
	files, err := collectFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatal(files)
	}
}
func TestShellQuote(t *testing.T) {
	input := "hello ' world; $(touch /tmp/sentinel-should-not-exist)"
	var output strings.Builder
	code, _, err := runCommand(context.Background(), t.TempDir(), "printf '%s\\n' "+shellQuote(input), func(_, s string) { output.WriteString(s) })
	if err != nil || code != 0 || output.String() != input {
		t.Fatalf("%v %d %s", err, code, output.String())
	}
}
