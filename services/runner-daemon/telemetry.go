package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var hostSample struct {
	sync.Mutex
	at         time.Time
	cpuSeconds float64
	percent    float64
}

func controllerCPU() float64 {
	hostSample.Lock()
	defer hostSample.Unlock()
	now := time.Now()
	if now.Sub(hostSample.at) < time.Second {
		return hostSample.percent
	}
	var r syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &r) != nil {
		return hostSample.percent
	}
	seconds := float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
	if !hostSample.at.IsZero() {
		hostSample.percent = 100 * (seconds - hostSample.cpuSeconds) / now.Sub(hostSample.at).Seconds()
	}
	hostSample.at = now
	hostSample.cpuSeconds = seconds
	return hostSample.percent
}
func workerTelemetry(index int, stamp string, policy SandboxPolicy) map[string]any {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	rss := memory.Sys
	if b, e := os.ReadFile("/sys/fs/cgroup/memory.current"); e == nil {
		if n, e := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); e == nil {
			rss = n
		}
	}
	return map[string]any{"id": env("HOSTNAME", "local") + ":" + strconv.Itoa(index), "host": env("HOSTNAME", "local"), "routine": index, "heartbeat": stamp, "last_heartbeat": stamp, "status": "online", "memory_bytes": rss, "cpu_percent": controllerCPU(), "metrics_scope": "controller_host", "sandbox_policy": policy, "sandbox_status": map[bool]string{true: "isolated", false: "unsafe"}[policy.Isolated], "sampled_at": time.Now().UTC().Format(time.RFC3339)}
}
