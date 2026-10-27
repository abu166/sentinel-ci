package main

import (
	"net/http"
	"time"
)

func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	v, e := a.query(r.Context(), `SELECT count(*) AS total_pipelines,count(*) FILTER(WHERE status='HEALED') AS auto_healed_bugs,coalesce(avg(extract(epoch FROM(finished_at-started_at))) FILTER(WHERE status='HEALED'),0)::float8 AS mttr_seconds,coalesce(100.0*count(*) FILTER(WHERE status='HEALED')/nullif(count(*) FILTER(WHERE status IN ('HEALED','FAILED')),0),0)::float8 AS healing_success_rate,coalesce(percentile_cont(.95) WITHIN GROUP(ORDER BY extract(epoch FROM(finished_at-started_at)) * 1000) FILTER(WHERE finished_at IS NOT NULL),0)::float8 AS p95_build_duration_ms,coalesce(avg(extract(epoch FROM(finished_at-started_at))*1000) FILTER(WHERE finished_at IS NOT NULL),0)::float8 AS build_duration_ms FROM pipelines`)
	if e != nil {
		fail(w, 500, e)
		return
	}
	workers, e := a.workerRecords(r.Context())
	if e != nil {
		fail(w, 503, e)
		return
	}
	q, e := a.queueInfo("jobs.dispatch")
	if e != nil {
		fail(w, 503, e)
		return
	}
	throughput, e := a.query(r.Context(), "SELECT date_trunc('day',started_at) AS day,count(*) AS count FROM pipelines WHERE started_at>=now()-interval '7 days' GROUP BY 1 ORDER BY 1")
	if e != nil {
		fail(w, 500, e)
		return
	}
	cpu, memory := aggregateWorkerUsage(workers)
	v[0]["active_workers"] = len(workers)
	v[0]["queue_depth"] = q[0]
	v[0]["throughput"] = throughput
	v[0]["worker_cpu_percent"] = cpu
	v[0]["worker_memory_bytes"] = memory
	now := time.Now().UTC()
	a.mu.Lock()
	if now.Sub(a.sampleAt) >= time.Second {
		a.telemetry = append(a.telemetry, map[string]any{"timestamp": now, "build_duration_ms": number(v[0]["build_duration_ms"]), "queue_depth": q[0], "cpu_percent": cpu, "memory_bytes": memory})
		a.sampleAt = now
		if len(a.telemetry) > 180 {
			a.telemetry = a.telemetry[len(a.telemetry)-180:]
		}
	}
	series := make([]map[string]any, len(a.telemetry))
	copy(series, a.telemetry)
	a.mu.Unlock()
	v[0]["timeseries"] = series
	write(w, 200, v[0])
}

func aggregateWorkerUsage(workers []map[string]any) (float64, float64) {
	cpu, memory := 0.0, 0.0
	seen := map[string]bool{}
	for _, worker := range workers {
		host, _ := worker["host"].(string)
		if host != "" && seen[host] {
			continue
		}
		if host != "" {
			seen[host] = true
		}
		cpu += number(worker["cpu_percent"])
		memory += number(worker["memory_bytes"])
	}
	return cpu, memory
}
