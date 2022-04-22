// Binary tgstatus servers telegram status page.
package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/go-faster/errors"
	"github.com/open2b/scriggo"
	"github.com/open2b/scriggo/native"
	"github.com/povilasv/prommod"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/gotd/td/telegram"

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

type Meta struct {
	City    string // Miami
	Country string // USA
	Name    string // pluto, venus
}

func run(ctx context.Context) error {
	lg, _ := zap.NewProduction(zap.IncreaseLevel(zapcore.DebugLevel))
	defer func() { _ = lg.Sync() }()

	status := tgstatus.New(telegram.TestAppID, telegram.TestAppHash, lg)

	meta := map[int]Meta{
		1: {
			Name:    "pluto",
			City:    "Miami",
			Country: "USA",
		},
		2: {
			Name:    "venus",
			City:    "Amsterdam",
			Country: "NL",
		},
		3: {
			Name:    "aurora",
			Country: "USA",
			City:    "Miami",
		},
		4: {
			Name:    "vesta",
			Country: "NL",
			City:    "Amsterdam",
		},
		5: {
			Name:    "flora",
			Country: "SG",
			City:    "Singapore",
		},
	}
	fs := scriggo.Files{"index.html": tgstatus.Web}
	tpl, err := scriggo.BuildTemplate(fs, "index.html", &scriggo.BuildOptions{
		Globals: native.Declarations{
			"Reports": status.Report,
			"Timeout": time.Minute,
			"Ago":     formatAgo,
			"Now":     time.Now,
			"Meta":    &meta,
		},
	})
	if err != nil {
		return errors.Wrap(err, "build template")
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		prommod.NewCollector("tgstatd"),
		status,
	)

	// Cache status page and update it in background.
	var page atomic.Value
	render := func() {
		buf := new(bytes.Buffer)
		if err := tpl.Run(buf, nil, nil); err != nil {
			lg.Error("Template run", zap.Error(err))
		}
		page.Store(buf.Bytes())
	}
	render()
	go func() {
		for range time.NewTicker(time.Second).C {
			render()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(page.Load().([]byte))
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
		lg.Warn("Serving metrics on public endpoint")
		attachProfiler(mux)
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	} else {
		// Serving metrics on different addr.
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		attachProfiler(metricsMux)
		metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux}
		groupServe(gCtx, lg.Named("http.metrics"), g, metricsServer)
	}

	server := &http.Server{Addr: httpAddr, Handler: mux}
	groupServe(gCtx, lg.Named("http"), g, server)

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
