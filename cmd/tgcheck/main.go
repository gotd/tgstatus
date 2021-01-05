package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram"
)

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

	client := telegram.NewClient(appID, appHash, telegram.Options{Logger: logger})
	return client.Run(ctx, func(ctx context.Context) error {
		logger.Info("Connected")
		return nil
	})
}

func main() {
	ctx, cancel := withSignal(context.Background())
	defer cancel()
	if err := run(ctx); err != nil && xerrors.Is(err, context.Canceled) {
		panic(err)
	}
}
