package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Resources struct {
	PeakRSSBytes int64         `json:"peak_rss_bytes"`
	CPUUserMS    int64         `json:"cpu_user_ms"`
	CPUSystemMS  int64         `json:"cpu_system_ms"`
	Policy       SandboxPolicy `json:"sandbox_policy"`
}
type SandboxPolicy struct {
	Backend           string  `json:"backend"`
	Isolated          bool    `json:"isolated"`
	ReadOnlyRoot      bool    `json:"read_only_root"`
	Network           string  `json:"network"`
	MemoryMB          int     `json:"memory_mb"`
	CPUs              float64 `json:"cpus"`
	Pids              int     `json:"pids_limit"`
	SecretsScrubbed   bool    `json:"secrets_scrubbed"`
	MeasurementSource string  `json:"measurement_source"`
}
type SandboxExecutor struct {
	Mode     string
	Image    string
	MemoryMB int
	CPUs     float64
	Pids     int
	Network  string
}

func sandboxConfig() (SandboxExecutor, error) {
	s := SandboxExecutor{Mode: env("SANDBOX_MODE", "docker"), Image: env("SANDBOX_IMAGE", "sentinel-sandbox:local"), MemoryMB: 768, CPUs: 1, Pids: 128, Network: env("SANDBOX_NETWORK", "none")}
	var err error
	if v := os.Getenv("SANDBOX_MEMORY_MB"); v != "" {
		s.MemoryMB, err = strconv.Atoi(v)
		if err != nil {
			return s, err
		}
	}
	if v := os.Getenv("SANDBOX_CPUS"); v != "" {
		s.CPUs, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return s, err
		}
	}
	if s.Mode != "docker" {
		return s, errors.New("production runner requires SANDBOX_MODE=docker; process mode is restricted to unit tests")
	}
	if s.MemoryMB < 128 || s.MemoryMB > 8192 || math.IsNaN(s.CPUs) || math.IsInf(s.CPUs, 0) || s.CPUs < 0.1 || s.CPUs > 8 || s.Network != "none" && s.Network != "bridge" {
		return s, errors.New("invalid sandbox resource/network policy")
	}
	return s, nil
}
func cleanEnvironment(root string) []string {
	return []string{"PATH=" + env("PATH", "/usr/local/bin:/usr/bin:/bin"), "HOME=" + root, "TMPDIR=" + os.TempDir(), "LANG=C.UTF-8", "GOTOOLCHAIN=local", "GOPROXY=off", "GOMAXPROCS=2", "GOFLAGS=-p=2", "GOCACHE=" + filepath.Join(root, ".gocache"), "GOPATH=" + filepath.Join(root, ".gopath"), "PYTHONDONTWRITEBYTECODE=1"}
}
func runProcess(ctx context.Context, dir, command string, input io.Reader, emit func(string, string)) (int, Resources, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = cleanEnvironment(dir)
	cmd.Stdin = input
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, Resources{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return -1, Resources{}, err
	}
	if err = cmd.Start(); err != nil {
		return -1, Resources{}, err
	}
	var wg sync.WaitGroup
	scan := func(reader io.Reader, stream string) {
		defer wg.Done()
		s := bufio.NewScanner(reader)
		s.Buffer(make([]byte, 4096), 1<<20)
		for s.Scan() {
			emit(stream, s.Text())
		}
		if err := s.Err(); err != nil {
			emit("stderr", "log reader: "+err.Error())
		}
	}
	wg.Add(2)
	go scan(stdout, "stdout")
	go scan(stderr, "stderr")
	wg.Wait()
	err = cmd.Wait()
	usage := Resources{}
	if cmd.ProcessState != nil {
		if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			usage.PeakRSSBytes = ru.Maxrss
			if runtime.GOOS != "darwin" {
				usage.PeakRSSBytes *= 1024
			}
			usage.CPUUserMS = ru.Utime.Sec*1000 + int64(ru.Utime.Usec)/1000
			usage.CPUSystemMS = ru.Stime.Sec*1000 + int64(ru.Stime.Usec)/1000
		}
	}
	if ctx.Err() != nil {
		return 124, usage, ctx.Err()
	}
	if err == nil {
		return 0, usage, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), usage, nil
	}
	return -1, usage, err
}
func runCommand(ctx context.Context, dir, command string, emit func(string, string)) (int, int64, error) {
	code, usage, err := runProcess(ctx, dir, command, nil, emit)
	return code, usage.PeakRSSBytes, err
}
func workspaceTar(root string) ([]byte, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	total := int64(0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".gocache" || d.Name() == ".gopath" || d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink forbidden: %s", rel)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular source file: %s", rel)
		}
		total += info.Size()
		if total > 64<<20 {
			return errors.New("sandbox source exceeds 64MB")
		}
		h := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0600, Size: info.Size(), Typeflag: tar.TypeReg}
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = io.Copy(tw, f)
		f.Close()
		return e
	})
	closeErr := tw.Close()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), closeErr
}
func volumeName(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "sentinel-work-" + hex.EncodeToString(sum[:12])
}
func (s SandboxExecutor) policy() SandboxPolicy {
	source := "os_rusage"
	if s.Mode == "docker" {
		source = "time_subprocess"
	}
	return SandboxPolicy{Backend: s.Mode, Isolated: s.Mode == "docker", ReadOnlyRoot: s.Mode == "docker", Network: s.Network, MemoryMB: s.MemoryMB, CPUs: s.CPUs, Pids: s.Pids, SecretsScrubbed: true, MeasurementSource: source}
}
func (s SandboxExecutor) dockerArgs(root, name, command string) []string {
	return []string{"run", "--rm", "--name", name, "--label", "sentinel.managed=true", "--label", "sentinel.controller=" + env("RUNNER_NODE_ID", env("HOSTNAME", "local")), "--label", "sentinel.volume=" + volumeName(root), "-i", "--read-only", "--network", s.Network, "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--memory", fmt.Sprintf("%dm", s.MemoryMB), "--memory-swap", fmt.Sprintf("%dm", s.MemoryMB), "--cpus", strconv.FormatFloat(s.CPUs, 'f', 2, 64), "--pids-limit", strconv.Itoa(s.Pids), "--ulimit", "nofile=256:256", "--ulimit", "core=0:0", "--ulimit", "fsize=536870912:536870912", "--ulimit", "cpu=120:120", "--user", "65532:65532", "--tmpfs", "/tmp:rw,exec,nosuid,nodev,size=536870912,uid=65532,gid=65532", "--mount", "type=volume,source=" + volumeName(root) + ",target=/workspace", "--workdir", "/workspace", "--env", "HOME=/tmp", "--env", "GOCACHE=/workspace/.gocache", "--env", "GOPATH=/workspace/.gopath", "--env", "GOTOOLCHAIN=local", "--env", "GOPROXY=off", "--env", "GOMAXPROCS=2", "--env", "GOFLAGS=-p=2", "--env", "PYTHONDONTWRITEBYTECODE=1", s.Image, "sh", "-c", "tar -xf - -C /workspace && /usr/bin/time -f '__SENTINEL_RESOURCE__ %M %U %S' sh -c " + shellQuote(command)}
}
func resourceLine(line string) (Resources, bool) {
	fields := strings.Fields(line)
	if len(fields) != 4 || fields[0] != "__SENTINEL_RESOURCE__" {
		return Resources{}, false
	}
	rss, e := strconv.ParseInt(fields[1], 10, 64)
	if e != nil || rss < 0 || rss > math.MaxInt64/1024 {
		return Resources{}, false
	}
	u, e := strconv.ParseFloat(fields[2], 64)
	if e != nil || u < 0 || math.IsNaN(u) || math.IsInf(u, 0) || u > float64(math.MaxInt64)/1000 {
		return Resources{}, false
	}
	sy, e := strconv.ParseFloat(fields[3], 64)
	if e != nil || sy < 0 || math.IsNaN(sy) || math.IsInf(sy, 0) || sy > float64(math.MaxInt64)/1000 {
		return Resources{}, false
	}
	return Resources{PeakRSSBytes: rss * 1024, CPUUserMS: int64(u * 1000), CPUSystemMS: int64(sy * 1000)}, true
}
func dockerControl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = cleanEnvironment(os.TempDir())
	return cmd.Run()
}
func (s SandboxExecutor) Execute(ctx context.Context, root, command string, emit func(string, string)) (int, Resources, error) {
	if s.Mode == "process" {
		code, u, err := runProcess(ctx, root, command, nil, emit)
		u.Policy = s.policy()
		return code, u, err
	}
	if s.Mode != "docker" {
		return -1, Resources{}, errors.New("unsupported sandbox backend")
	}
	archive, err := workspaceTar(root)
	if err != nil {
		return -1, Resources{}, err
	}
	name := "sentinel-step-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	args := s.dockerArgs(root, name, command)
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	defer dockerControl("rm", "-f", name)
	var usage Resources
	var mu sync.Mutex
	code, _, err := runProcess(ctx, root, "docker "+strings.Join(quoted, " "), bytes.NewReader(archive), func(stream, line string) {
		mu.Lock()
		defer mu.Unlock()
		if resource, ok := resourceLine(line); ok && stream == "stderr" {
			usage = resource
			return
		}
		emit(stream, line)
	})
	usage.Policy = s.policy()
	return code, usage, err
}
func (s SandboxExecutor) Cleanup(root string) {
	if s.Mode == "docker" {
		_ = dockerControl("volume", "rm", volumeName(root))
	}
}

// Reap removes interrupted containers owned by this controller before accepting work.
func (s SandboxExecutor) Reap(ctx context.Context) error {
	if s.Mode != "docker" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=sentinel.managed=true", "--filter", "label=sentinel.controller="+env("RUNNER_NODE_ID", env("HOSTNAME", "local")))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("sandbox daemon unavailable: %w", err)
	}
	for _, id := range strings.Fields(string(output)) {
		if err = dockerControl("rm", "-f", id); err != nil {
			return err
		}
	}
	return nil
}
