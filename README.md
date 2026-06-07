# obskit — Go Observability Toolkit

A zero-ceremony observability library for Go services.  
One import. Five packages. Production-ready out of the box.

```
go get github.com/incogni23/obskit
```

---

## What's inside

| Package | What it does |
|---|---|
| `logger` | Structured, leveled logging (zap) with context propagation; auto-attaches `correlation_id`, `trace_id`, `span_id` |
| `correlation` | Correlation-ID injection via `X-Correlation-ID` header |
| `tracing` | Lightweight per-request spans with W3C `traceparent` inbound extraction and outbound injection |
| `metrics` | Prometheus counters, gauges, histograms, and `/metrics` handler |
| `health` | Composable liveness (`/healthz`) and readiness (`/readyz`) probes |
| `middleware` | Inbound HTTP middleware chain + outbound `Transport` for cross-service propagation |

---

## Quick start

```go
import "github.com/incogni23/obskit"

obs := obskit.New(obskit.Config{
    LogLevel: "info",
    // LogJSON defaults to true (JSON lines). Opt out for local dev:
    // LogJSON: obskit.LogJSON(false),
})
defer obs.Logger.Sync()

// Register health checks
obs.Health.Register("database", func(ctx context.Context) health.CheckResult {
    if err := db.PingContext(ctx); err != nil {
        return health.CheckResult{Status: health.StatusDown, Message: err.Error()}
    }
    return health.CheckResult{Status: health.StatusUp}
})

// Wire up routes
mux := http.NewServeMux()
mux.Handle("/metrics", obs.Metrics.HTTPHandler())
mux.Handle("/healthz", obs.Health.LivenessHandler())
mux.Handle("/readyz",  obs.Health.ReadinessHandler())
mux.HandleFunc("/hello", myHandler)

// Single call wraps the mux with correlation + tracing + logging + metrics
http.ListenAndServe(":8080", obs.Middleware(mux))
```

Every request automatically gets:
- A `X-Correlation-ID` header (generated if missing, echoed in response)
- A trace span with `trace_id`, `span_id`
- A structured log line: method, path, status, duration, correlation ID
- Prometheus counters and duration histograms

---

## Packages

### logger

```go
log, _ := logger.New(logger.Config{Level: "debug", JSON: true, AddCaller: true})

// Store in context, retrieve downstream
ctx = log.WithContext(ctx)
logger.FromContext(ctx).Info("something happened", zap.String("key", "value"))
```

### correlation

```go
// Incoming request — extracts or generates ID
id, r := correlation.FromRequest(r)

// Outgoing call — propagate to downstream service
correlation.InjectHeader(ctx, req.Header)
```

### tracing

```go
ctx, span := tracing.Start(ctx, "process-payment")
defer span.End()

span.SetTag("user_id", userID)
// on error:
span.Finish(err)
```

### metrics

```go
reg := metrics.New()

requests := reg.Counter("api_calls_total", "Total API calls", "endpoint", "status")
latency  := reg.Histogram("api_latency_seconds", "API latency", nil, "endpoint")

requests.WithLabelValues("/checkout", "200").Inc()
latency.WithLabelValues("/checkout").Observe(0.032)

http.Handle("/metrics", reg.HTTPHandler())
```

### health

```go
h := health.New(5 * time.Second)

h.Register("redis", func(ctx context.Context) health.CheckResult {
    if err := rdb.Ping(ctx).Err(); err != nil {
        return health.CheckResult{Status: health.StatusDown, Message: err.Error()}
    }
    return health.CheckResult{Status: health.StatusUp}
})

http.Handle("/healthz", h.LivenessHandler())   // always 200
http.Handle("/readyz",  h.ReadinessHandler())  // 200 or 503
```

Readiness response:

```json
{
  "status": "up",
  "checks": {
    "redis": { "status": "up", "latency_ms": 1000000 }
  }
}
```

---

## Outbound propagation

Wrap your HTTP clients so `X-Correlation-ID` and `traceparent` flow to downstream services:

```go
client := &http.Client{
    Transport: middleware.Transport(nil), // nil = http.DefaultTransport
}
```

The transport reads both values from the request context, so no manual header plumbing is needed.

---

## Kubernetes probe config

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
```

---

## License

MIT
