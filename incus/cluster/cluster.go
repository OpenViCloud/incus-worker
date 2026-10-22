// Package cluster resolves a zone to a connected Incus cluster through the control plane
// (https://github.com/OpenViCloud/titan-ops/blob/main/docs/features/cluster/design.md#worker-cluster-connection), caching the connection for a few minutes.
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/OpenViCloud/titan-incus-worker/controlplane"
	"github.com/OpenViCloud/titan-incus-worker/incus/provider"
)

// ErrNoCluster: the zone has no cluster row (404 CLUSTER_NOT_FOUND). A command for it cannot succeed by retrying.
var ErrNoCluster error = &noClusterError{}

type noClusterError struct{}

func (e *noClusterError) Error() string {
	return "zone has no cluster"
}

// Permanent tells the command loop not to retry, without this package importing it.
func (e *noClusterError) Permanent() bool {
	return true
}

const cacheFor = 5 * time.Minute

// Zone is one connected cluster with the zone's provider settings ("" = not set).
type Zone struct {
	Provider         provider.Provider
	OvnUplinkNetwork string
	StoragePool      string
}

// connectionResponse mirrors the control plane's ClusterConnectionResponse. Never logged: it carries the private key.
type connectionResponse struct {
	Endpoint             string  `json:"endpoint"`
	ServerFingerprint    string  `json:"serverFingerprint"`
	ClientCertificatePem string  `json:"clientCertificatePem"`
	ClientKeyPem         string  `json:"clientKeyPem"`
	OvnUplinkNetwork     *string `json:"ovnUplinkNetwork"`
	StoragePool          *string `json:"storagePool"`
}

// Zones resolves a zone to its cluster; Resolver in production, Fixed in tests.
type Zones interface {
	Zone(ctx context.Context, zoneId string) (*Zone, error)
}

// Fixed serves preset zones; any other zone has no cluster.
type Fixed map[string]*Zone

func (f Fixed) Zone(ctx context.Context, zoneId string) (*Zone, error) {
	zone, found := f[zoneId]
	if !found {
		return nil, fmt.Errorf("zone %s: %w", zoneId, ErrNoCluster)
	}
	return zone, nil
}

type ConnectFunc func(provider.Credentials) (provider.Provider, error)

// ConnectIncus is the production ConnectFunc.
func ConnectIncus(credentials provider.Credentials) (provider.Provider, error) {
	server, err := provider.Connect(credentials)
	if err != nil {
		return nil, err
	}
	return provider.NewIncus(server), nil
}

type cached struct {
	zone      *Zone
	expiresAt time.Time
}

type Resolver struct {
	cp      *controlplane.Client
	connect ConnectFunc
	logger  *slog.Logger
	now     func() time.Time

	mu    sync.Mutex
	zones map[string]cached
}

func NewResolver(cp *controlplane.Client, connect ConnectFunc, logger *slog.Logger) *Resolver {
	return &Resolver{cp: cp, connect: connect, logger: logger, now: time.Now, zones: map[string]cached{}}
}

// Zone returns the zone's cluster, connecting on a cache miss. errors.Is(err, ErrNoCluster) when the zone has none.
func (r *Resolver) Zone(ctx context.Context, zoneId string) (*Zone, error) {
	r.mu.Lock()
	entry, found := r.zones[zoneId]
	r.mu.Unlock()
	isFresh := found && r.now().Before(entry.expiresAt)
	if isFresh {
		return entry.zone, nil
	}

	zone, err := r.load(ctx, zoneId)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.zones[zoneId] = cached{zone: zone, expiresAt: r.now().Add(cacheFor)}
	r.mu.Unlock()
	return zone, nil
}

func (r *Resolver) load(ctx context.Context, zoneId string) (*Zone, error) {
	var connection connectionResponse
	err := r.cp.GetJSON(ctx, "/internal/clusters/"+url.PathEscape(zoneId)+"/connection", &connection)
	if controlplane.StatusCodeOf(err) == http.StatusNotFound {
		return nil, fmt.Errorf("zone %s: %w (%w)", zoneId, ErrNoCluster, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read cluster connection of zone %s: %w", zoneId, err)
	}

	connected, err := r.connect(provider.Credentials{
		Endpoint:             connection.Endpoint,
		ServerFingerprint:    connection.ServerFingerprint,
		ClientCertificatePem: connection.ClientCertificatePem,
		ClientKeyPem:         connection.ClientKeyPem,
	})
	if err != nil {
		return nil, fmt.Errorf("zone %s: %w", zoneId, err)
	}
	// The connection is kept for cacheFor: a cluster re-trusted or moved meanwhile is reached only after it runs out.
	r.logger.Info("connected to the zone's cluster", "zoneId", zoneId, "endpoint", connection.Endpoint,
		"cachedFor", cacheFor.String())
	return &Zone{
		Provider:         connected,
		OvnUplinkNetwork: valueOrEmpty(connection.OvnUplinkNetwork),
		StoragePool:      valueOrEmpty(connection.StoragePool),
	}, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
