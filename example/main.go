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
		LogJSON:   obskit.LogJSON(false),
		LogCaller: true,
	})
	defer obs.Logger.Sync()

	obs.Health.Register("database", func(ctx context.Context) health.CheckResult {
		return health.CheckResult{Status: health.StatusUp}
	})
	obs.Health.Register("cache", health.OK)

	mux := http.NewServeMux()

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.Start(r.Context(), "hello-handler")
		defer span.End()
		logger.FromContext(ctx).Info("handling hello request")
		fmt.Fprintln(w, "Hello from obskit!")
	})

	mux.Handle("/metrics", obs.Metrics.HTTPHandler())
	mux.Handle("/healthz", obs.Health.LivenessHandler())
	mux.Handle("/readyz", obs.Health.ReadinessHandler())

	obs.Logger.Info("listening on :8080")
	if err := http.ListenAndServe(":8080", obs.Middleware(mux)); err != nil {
		log.Fatal(err)
	}
}
