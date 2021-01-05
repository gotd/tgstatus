// Binary tgstatus servers telegram status page.
package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/gotd/td/session"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type DC struct {
	ID   int
	IP   net.IP
	CDN  bool
	Port int
}

func (d DC) Addr() string {
	return net.JoinHostPort(d.IP.String(), strconv.Itoa(d.Port))
}

type sessionStorage struct {
	mux      sync.Mutex
	deadline time.Time
	data     []byte
}

func (s *sessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if len(s.data) == 0 || time.Now().After(s.deadline) {
		return nil, session.ErrNotFound
	}
	return s.data, nil
}

func (s *sessionStorage) StoreSession(ctx context.Context, data []byte) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.data = data
	s.deadline = time.Now().Add(time.Minute)
	return nil
}

type Server struct {
	cfg tg.Config
	log *zap.Logger

	appID    int
	appHash  string
	mux      sync.Mutex
	sessions map[int]*sessionStorage

	stats *State
}

type ipStat struct {
	ip   net.IP
	seen time.Time
}

type dcStat struct {
	id      int
	seen    time.Time
	latency time.Duration
	ips     map[string]ipStat
}

type Metric struct {
	Seen    time.Time
	Latency time.Duration
	Server  DC
}

type State struct {
	mux    sync.Mutex
	status map[int]dcStat
}

type Status struct {
	DC       int
	LastSeen time.Time
	Addr     []net.IP
	Latency  time.Duration
}

// Status return slice of current datacenter statuses.
func (s *State) Status() []Status {
	s.mux.Lock()
	defer s.mux.Unlock()

	var statuses []Status
	for _, v := range s.status {
		status := Status{
			DC:       v.id,
			LastSeen: v.seen,
			Latency:  v.latency,
		}
		for _, addr := range v.ips {
			status.Addr = append(status.Addr, addr.ip)
		}
		sort.SliceStable(status.Addr, func(i, j int) bool {
			return bytes.Compare(status.Addr[i], status.Addr[j]) < 0
		})
		statuses = append(statuses, status)
	}

	sort.SliceStable(statuses, func(i, j int) bool {
		return statuses[i].DC < statuses[j].DC
	})

	return statuses
}

func (s *State) Consume(m Metric) {
	s.mux.Lock()
	defer s.mux.Unlock()

	stat := s.status[m.Server.ID]
	stat.latency = m.Latency
	stat.seen = m.Seen
	stat.id = m.Server.ID

	{
		if stat.ips == nil {
			stat.ips = map[string]ipStat{}
		}
		ip := stat.ips[m.Server.Addr()]
		ip.seen = m.Seen
		ip.ip = m.Server.IP
		stat.ips[m.Server.Addr()] = ip
	}

	s.status[m.Server.ID] = stat
}

func (s *Server) DataCenters() []DC {
	var dataCenters []DC
	for _, dc := range s.cfg.DCOptions {
		s.log.With(
			zap.Bool("ipv6", dc.Ipv6),
			zap.Int("dc", dc.ID),
			zap.String("addr", dc.IPAddress),
			zap.Bool("cdn", dc.CDN),
			zap.Bool("static", dc.Static),
			zap.Bool("obfuscated", dc.TcpoOnly),
			zap.Int("port", dc.Port),
		).Info("Datacenter")

		// Skipping ipv6, obfuscated and proxy-only.
		if dc.Ipv6 || dc.TcpoOnly || dc.Static {
			continue
		}

		dataCenters = append(dataCenters, DC{
			IP:   net.ParseIP(dc.IPAddress),
			CDN:  dc.CDN,
			ID:   dc.ID,
			Port: dc.Port,
		})
	}
	return dataCenters
}

func (s *Server) Check(ctx context.Context, dc DC) error {
	s.mux.Lock()
	sess := s.sessions[dc.ID]
	if sess == nil {
		sess = &sessionStorage{}
		s.sessions[dc.ID] = sess
	}
	s.mux.Unlock()

	start := time.Now()
	log := s.log.With(
		zap.Int("check_dc", dc.ID),
		zap.Int64("check_id", start.Unix()*10+int64(dc.ID)),
	)

	if err := backoff.Retry(func() error {
		client := telegram.NewClient(s.appID, s.appHash, telegram.Options{
			Logger:         log,
			SessionStorage: sess,
			Addr:           dc.Addr(),
		})
		log.Debug("Connecting")
		return client.Run(ctx, func(ctx context.Context) error {
			latency := time.Since(start)
			log.Info("Connected", zap.Duration("latency", latency))
			s.stats.Consume(Metric{
				Server:  dc,
				Seen:    start,
				Latency: latency,
			})

			return nil
		})
	}, backoff.NewExponentialBackOff()); err != nil {
		s.stats.Consume(Metric{
			Server: dc,
		})
		return xerrors.Errorf("connect: %w", err)
	}

	return nil
}

func (s *Server) CheckAll(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	g, gCtx := errgroup.WithContext(ctx)
	for _, dc := range s.DataCenters() {
		if dc.IP.To4() == nil {
			continue
		}
		if dc.CDN {
			continue
		}

		currentDC := dc
		g.Go(func() error {
			return s.Check(gCtx, currentDC)
		})
	}

	return g.Wait()
}

func (s *Server) Init(ctx context.Context) error {

	if err := backoff.Retry(func() error {
		client := telegram.NewClient(s.appID, s.appHash, telegram.Options{
			Logger: s.log,
		})

		return client.Run(ctx, func(ctx context.Context) error {
			cfg, err := tg.NewClient(client).HelpGetConfig(ctx)
			if err != nil {
				return xerrors.Errorf("config: %w", err)
			}
			s.cfg = *cfg
			return nil
		})
	}, backoff.NewExponentialBackOff()); err != nil {
		return xerrors.Errorf("connect: %w", err)
	}

	return nil
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

	server := &Server{
		log:      logger,
		appID:    appID,
		appHash:  appHash,
		sessions: map[int]*sessionStorage{},

		stats: &State{
			status: map[int]dcStat{},
		},
	}

	if err := server.Init(ctx); err != nil {
		return xerrors.Errorf("init: %w", err)
	}

	go func() {
		// Send initial check
		if err := server.CheckAll(ctx); err != nil {
			logger.Error("Failed to check", zap.Error(err))
		}

		ticker := time.NewTicker(time.Second * 30)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := server.CheckAll(ctx); err != nil {
					logger.Error("Failed to check", zap.Error(err))
				}
			}
		}
	}()

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		now := time.Now()
		deadline := now.Add(-time.Second * 60)

		stats := server.stats.Status()
		if len(stats) == 0 {
			fmt.Fprintln(w, "No stats available")
		}

		for _, dc := range stats {
			if dc.LastSeen.After(deadline) {
				_, _ = fmt.Fprintf(w, "DC %02d: UP (lag %8s) %s\n",
					dc.DC, dc.Latency.Round(time.Millisecond), dc.Addr,
				)
			} else {
				_, _ = fmt.Fprintf(w, "DC %02d: DOWN (%8s ago) %s\n",
					dc.DC, now.Sub(dc.LastSeen).Round(time.Second), dc.Addr,
				)
			}
		}
	})

	return http.ListenAndServe(addr, mux)
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		panic(err)
	}
}
