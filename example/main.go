package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/incogni23/obskit"
	"github.com/incogni23/obskit/health"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/tracing"
)

func main() {
	obs := obskit.New(obskit.Config{
		LogLevel:  "debug",
		LogJSON:   obskit.LogJSON(false), // pretty console for local dev
		LogCaller: true,
	})
	defer obs.Logger.Sync()

	// --- Health checks ---
	obs.Health.Register("database", func(ctx context.Context) health.CheckResult {
		// replace with a real ping, e.g. db.PingContext(ctx)
		return health.CheckResult{Status: health.StatusUp, Message: "ok"}
	})
	obs.Health.Register("cache", health.OK)

	// --- Routes ---
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// Store the obs logger in ctx so logger.FromContext picks it up;
		// the middleware has already injected correlation_id and trace_id.
		ctx := obs.Logger.WithContext(r.Context())

		_, span := tracing.Start(ctx, "hello-handler")
		defer span.End()

		// FromContext enriches the log line with correlation_id and trace_id
		// automatically via the extractors registered in obskit.New.
		logger.FromContext(ctx).Info("handling hello request")

		fmt.Fprintln(w, "Hello from obskit!")
	})

	mux.Handle("/metrics", obs.Metrics.HTTPHandler())
	mux.Handle("/healthz", obs.Health.LivenessHandler())
	mux.Handle("/readyz", obs.Health.ReadinessHandler())

	handler := obs.Middleware(mux)

	obs.Logger.Info("server starting on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
