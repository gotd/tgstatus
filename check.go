package tgstatus

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type Check struct {
	mux     sync.Mutex
	appID   int
	appHash string
	rate    time.Duration
	id      int
	addr    string
	log     *zap.Logger
	seen    time.Time
}

type Report struct {
	ID   int
	Addr string
	Seen time.Time
}

func (c *Check) Report() Report {
	c.mux.Lock()
	defer c.mux.Unlock()
	return Report{
		ID:   c.id,
		Addr: c.addr,
		Seen: c.seen,
	}
}

func (c *Check) checkConnection(ctx context.Context, invoker tg.Invoker) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	if _, err := tg.NewClient(invoker).HelpGetConfig(ctx); err != nil {
		return err
	}

	// Success.
	c.seen = time.Now()

	return nil
}

func (c *Check) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.rate)
	client := telegram.NewClient(c.appID, c.appHash, telegram.Options{
		Addr:   c.addr,
		Logger: c.log,
	})
	return client.Run(ctx, func(ctx context.Context) error {
		for {
			select {
			case <-ticker.C:
				if err := c.checkConnection(ctx, client); err != nil {
					return xerrors.Errorf("check: %w", err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
}

func (c *Check) Loop(ctx context.Context) error {
	for {
		if err := c.Run(ctx); err != nil {
			c.log.Error("Run", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}
}
