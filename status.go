package tgstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type Status struct {
	appID   int
	appHash string
	log     *zap.Logger
	mux     sync.Mutex
	checks  []*Check
}

func (s *Status) Report() []Report {
	var reports []Report

	s.mux.Lock()
	for _, c := range s.checks {
		reports = append(reports, c.Report())
	}
	s.mux.Unlock()

	return reports
}

func (s *Status) config(ctx context.Context) (*tg.Config, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		client := telegram.NewClient(s.appID, s.appHash, telegram.Options{
			Logger: s.log.Named("client.initial"),
		})
		cfg := make(chan *tg.Config, 1)
		s.log.Debug("Getting config")
		if err := client.Run(ctx, func(ctx context.Context) error {
			s.log.Debug("Started client")
			gotCfg, err := tg.NewClient(client).HelpGetConfig(ctx)
			if err != nil {
				s.log.Debug("Got error on HelpGetConfig", zap.Error(err))
				return err
			}
			s.log.Debug("Got config")
			cfg <- gotCfg
			return nil
		}); err != nil {
			close(cfg)
			continue
		}

		s.log.Debug("Waiting for config or context done")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case gotConfig := <-cfg:
			return gotConfig, nil
		}
	}
}

func (s *Status) Run(ctx context.Context) error {
	cfg, err := s.config(ctx)
	if err != nil {
		return err
	}
	g, gCtx := errgroup.WithContext(ctx)

	s.mux.Lock()
	for _, dc := range cfg.DCOptions {
		if dc.Ipv6 || dc.TcpoOnly || dc.Static {
			continue
		}

		check := &Check{
			appID:   s.appID,
			appHash: s.appHash,
			rate:    time.Second * 10,
			id:      dc.ID,
			addr:    fmt.Sprintf("%s:%d", dc.IPAddress, dc.Port),
			log: s.log.With(
				zap.Int("dc", dc.ID),
				zap.String("addr", dc.IPAddress),
			),
		}
		s.checks = append(s.checks, check)
		g.Go(func() error {
			return check.Loop(gCtx)
		})
	}
	s.mux.Unlock()

	return g.Wait()
}

func New(appID int, appHash string, log *zap.Logger) *Status {
	return &Status{
		appID:   appID,
		appHash: appHash,
		log:     log,
	}
}
