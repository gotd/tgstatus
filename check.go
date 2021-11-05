package tgstatus

import (
	"context"
	"sync"
	"time"

	"github.com/ogen-go/errors"
	"go.uber.org/zap"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
)

type Check struct {
	mux     sync.Mutex
	appID   int
	appHash string
	rate    time.Duration
	id      int
	ip      string
	option  tg.DCOption
	port    int
	log     *zap.Logger
	seen    time.Time
}

type Report struct {
	ID   int
	IP   string
	Seen time.Time
}

func (c *Check) Report() Report {
	c.mux.Lock()
	defer c.mux.Unlock()
	return Report{
		ID:   c.id,
		IP:   c.ip,
		Seen: c.seen,
	}
}

func (c *Check) updateAddrFromConfig(cfg *tg.Config) {
	for _, dc := range cfg.DCOptions {
		if dc.Ipv6 || dc.TCPObfuscatedOnly || dc.Static || dc.MediaOnly {
			continue
		}
		if dc.ID != c.id {
			continue
		}

		c.mux.Lock()
		if c.ip != dc.IPAddress {
			c.log.Debug("Updating addr",
				zap.String("addr_old", c.ip),
				zap.String("addr_new", dc.IPAddress),
			)
			c.ip = dc.IPAddress
			c.option = dc
			c.port = dc.Port
		}
		c.mux.Unlock()

		break
	}
}

func (c *Check) checkConnection(ctx context.Context, invoker tg.Invoker) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	cfg, err := tg.NewClient(invoker).HelpGetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getConfig")
	}

	// IP can change over time.
	c.updateAddrFromConfig(cfg)

	// Success.
	c.mux.Lock()
	c.seen = time.Now()
	c.mux.Unlock()

	return nil
}

func (c *Check) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.rate)
	c.mux.Lock()
	client := telegram.NewClient(c.appID, c.appHash, telegram.Options{
		Logger: c.log,
		DC:     c.id,
		DCList: dcs.List{
			Options: []tg.DCOption{
				c.option,
			},
		},
	})
	c.mux.Unlock()
	return client.Run(ctx, func(ctx context.Context) error {
		for {
			select {
			case <-ticker.C:
				if err := c.checkConnection(ctx, client); err != nil {
					return errors.Wrap(err, "check")
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
