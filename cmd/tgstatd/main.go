// Binary tgstatus servers telegram status page.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/go-faster/errors"
	"github.com/povilasv/prommod"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/gotd/tgstatus"
)

func formatAgo(now, seen time.Time) string {
	if seen.IsZero() {
		return "long time"
	}
	return now.Sub(seen).Round(time.Second).String()
}

func groupServe(ctx context.Context, log *zap.Logger, g *errgroup.Group, server *http.Server) {
	g.Go(func() error {
		log.Info("ListenAndServe", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		log.Debug("Shutting down")
		return server.Close()
	})
}

func attachProfiler(router *http.ServeMux) {
	router.HandleFunc("/status", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})

	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	// Manually add support for paths linked to by index page at /debug/pprof/
	router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	router.Handle("/debug/pprof/block", pprof.Handler("block"))
}

func run(ctx context.Context) error {
	logger, _ := zap.NewDevelopment(zap.IncreaseLevel(zapcore.DebugLevel))
	defer func() { _ = logger.Sync() }()

	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return errors.Wrap(err, "APP_ID not set or invalid")
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return errors.New("no APP_HASH provided")
	}

	status := tgstatus.New(appID, appHash, logger)

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		prommod.NewCollector("tgstatd"),
		status,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		deadline := now.Add(-time.Second * 60)

		var b bytes.Buffer

		reports := status.Report()
		if len(reports) == 0 {
			b.WriteString("No stats available")
		}

		for _, dc := range reports {
			if dc.Seen.After(deadline) {
				fmt.Fprintf(&b, "DC %02d: UP %s\n",
					dc.ID, dc.IP,
				)
			} else {
				fmt.Fprintf(&b, "DC %02d: DOWN (%8s ago) %s\n",
					dc.ID, formatAgo(now, dc.Seen), dc.IP,
				)
			}
		}

		_, _ = b.WriteTo(w)
	})

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return status.Run(gCtx) })

	// Setting up http servers.
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "localhost:8081"
	}

	if metricsAddr == httpAddr {
		// Serving metrics on same addr.
		logger.Warn("Serving metrics on public endpoint")
		attachProfiler(mux)
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	} else {
		// Serving metrics on different addr.
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		attachProfiler(metricsMux)
		metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
		groupServe(gCtx, logger.Named("http.metrics"), g, metricsServer)
	}

	server := &http.Server{Addr: httpAddr, Handler: mux}
	groupServe(gCtx, logger.Named("http"), g, server)

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		panic(err)
	}
}
