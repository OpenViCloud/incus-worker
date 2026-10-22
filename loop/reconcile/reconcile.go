// Package reconcile is the reconciler loop: every interval, each kind in each zone that has a cluster
// (https://github.com/OpenViCloud/titan-ops/blob/main/docs/architecture/provider-and-worker.md#reconciler-is-a-worker-ticker). An error skips that zone and kind for the
// cycle; the loop never exits on one.
package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/OpenViCloud/titan-incus-worker/controlplane"
	"github.com/OpenViCloud/titan-incus-worker/incus/cluster"
)

// zoneInfo mirrors the control plane's ZoneInfo on GET /v1/zones.
type zoneInfo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// Kind is one resource's reconciler. main.go registers them, so this loop knows no resource.
type Kind struct {
	Name      string
	Reconcile func(ctx context.Context, cp *controlplane.Client, zone *cluster.Zone, zoneId string, logger *slog.Logger) error
}

type Runner struct {
	cp       *controlplane.Client
	zones    cluster.Zones
	interval time.Duration
	logger   *slog.Logger
	// kinds run in this order in each zone.
	kinds []Kind

	// Zones already logged as having no cluster, so a zone without one is not logged every cycle.
	loggedWithoutCluster map[string]bool
}

func NewRunner(cp *controlplane.Client, zones cluster.Zones, kinds []Kind, interval time.Duration, logger *slog.Logger) *Runner {
	return &Runner{
		cp:                   cp,
		zones:                zones,
		interval:             interval,
		logger:               logger,
		kinds:                kinds,
		loggedWithoutCluster: map[string]bool{},
	}
}

// Run reconciles at once, then every interval, until ctx ends.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		r.Cycle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Cycle runs every kind once in every zone.
func (r *Runner) Cycle(ctx context.Context) {
	var zones []zoneInfo
	if err := r.cp.GetPublicJSON(ctx, "/v1/zones", &zones); err != nil {
		if ctx.Err() == nil {
			r.logger.Warn("reconcile: listing zones failed; skipping this cycle", "error", err)
		}
		return
	}
	for _, zone := range zones {
		if ctx.Err() != nil {
			return
		}
		r.reconcileZone(ctx, zone.Id)
	}
}

func (r *Runner) reconcileZone(ctx context.Context, zoneId string) {
	zone, err := r.zones.Zone(ctx, zoneId)
	if errors.Is(err, cluster.ErrNoCluster) {
		if !r.loggedWithoutCluster[zoneId] {
			r.logger.Info("reconcile: zone has no cluster; skipping it until it has one", "zoneId", zoneId)
			r.loggedWithoutCluster[zoneId] = true
		}
		return
	}
	if err != nil {
		r.logger.Warn("reconcile: zone's cluster unreachable; skipping it this cycle", "zoneId", zoneId, "error", err)
		return
	}
	delete(r.loggedWithoutCluster, zoneId)

	// One line per zone and cycle says how long each kind took: a kind stuck on a slow Incus read shows here.
	zoneStartedAt := time.Now()
	durations := []any{}
	for _, kind := range r.kinds {
		kindLogger := r.logger.With("kind", kind.Name, "zoneId", zoneId)
		kindStartedAt := time.Now()
		err := kind.Reconcile(ctx, r.cp, zone, zoneId, kindLogger)
		durations = append(durations, slog.Int64(kind.Name, time.Since(kindStartedAt).Milliseconds()))
		if err != nil && ctx.Err() == nil {
			kindLogger.Warn("reconcile failed; skipping it this cycle", "error", err)
		}
	}
	r.logger.Info("reconcile zone done", "zoneId", zoneId, "totalMs", time.Since(zoneStartedAt).Milliseconds(),
		slog.Group("durationsMs", durations...))
}
