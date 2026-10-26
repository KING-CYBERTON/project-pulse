package server

import (
	"context"
	"sync"
	"time"

	"github.com/KING-CYBERTON/project-pulse/internal/analytics"
	"github.com/KING-CYBERTON/project-pulse/internal/config"
	gh "github.com/KING-CYBERTON/project-pulse/internal/github"
)

// Snapshot is one complete analysis of every configured project.
type Snapshot struct {
	At        time.Time
	Took      time.Duration
	Reports   []analytics.Report
	Portfolio analytics.Portfolio
}

// Service fetches and caches snapshots.
type Service struct {
	cfg    *config.Config
	client *gh.Client
	now    func() time.Time

	refresh sync.Mutex   // serialises refreshes so concurrent requests share one fetch
	mu      sync.RWMutex // guards snap
	snap    *Snapshot
}

// NewService builds a service. now may be nil (uses time.Now).
func NewService(cfg *config.Config, client *gh.Client, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{cfg: cfg, client: client, now: now}
}

// Snapshot returns the cached snapshot, refreshing it when older than the
// TTL or when force is set.
func (s *Service) Snapshot(ctx context.Context, force bool) (*Snapshot, error) {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	if cur := s.Cached(); cur != nil && !force && s.now().Sub(cur.At) < s.cfg.CacheTTL {
		return cur, nil
	}
	start := s.now()
	since := start.AddDate(0, 0, -s.cfg.LookbackDays)

	reports := make([]analytics.Report, len(s.cfg.Projects))
	var wg sync.WaitGroup
	for i, p := range s.cfg.Projects {
		wg.Add(1)
		go func(i int, p config.Project) {
			defer wg.Done()
			d, err := s.client.FetchRepo(ctx, p.Repo, since)
			if err != nil {
				reports[i] = analytics.Failed(p, err, s.cfg.LookbackDays, start)
				return
			}
			reports[i] = analytics.Analyze(p, d, s.cfg.LookbackDays, start)
		}(i, p)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		if cur := s.Cached(); cur != nil {
			return cur, nil // client went away; keep the old snapshot
		}
		return nil, err
	}
	snap := &Snapshot{
		At:        start,
		Took:      s.now().Sub(start),
		Reports:   reports,
		Portfolio: analytics.BuildPortfolio(reports, s.cfg.LookbackDays, start),
	}
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
	return snap, nil
}

// Cached returns the last snapshot without fetching (may be nil).
func (s *Service) Cached() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}
