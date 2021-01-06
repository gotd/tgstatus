// Binary tgstatus servers telegram status page.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/gotd/tgstatus"
)

func formatAgo(now, seen time.Time) string {
	if seen.IsZero() {
		return "long time"
	}
	return now.Sub(seen).Round(time.Second).String()
}

func run(ctx context.Context) error {
	logger, _ := zap.NewDevelopment(zap.IncreaseLevel(zapcore.DebugLevel))
	defer func() { _ = logger.Sync() }()

	// Reading app id from env (never hardcode it!).
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return xerrors.Errorf("APP_ID not set or invalid: %w", err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return xerrors.New("no APP_HASH provided")
	}

	status := tgstatus.New(appID, appHash, logger)

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		now := time.Now()
		deadline := now.Add(-time.Second * 60)

		reports := status.Report()
		if len(reports) == 0 {
			fmt.Fprintln(w, "No stats available")
		}

		for _, dc := range reports {
			if dc.Seen.After(deadline) {
				fmt.Fprintf(w, "DC %02d: UP %s\n",
					dc.ID, dc.Addr,
				)
			} else {
				fmt.Fprintf(w, "DC %02d: DOWN (%8s ago) %s\n",
					dc.ID, formatAgo(now, dc.Seen), dc.Addr,
				)
			}
		}
	})

	server := &http.Server{Addr: httpAddr, Handler: mux}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return status.Run(gCtx) })
	g.Go(func() error { return server.ListenAndServe() })
	g.Go(func() error {
		<-gCtx.Done()
		logger.Debug("Shutting down")
		return server.Close()
	})

	return g.Wait()
}

func withSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}

func main() {
	ctx, cancel := withSignal(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		panic(err)
	}
}
