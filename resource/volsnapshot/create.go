package volsnapshot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/OpenViCloud/incus-worker/controlplane"
	"github.com/OpenViCloud/incus-worker/incus/cluster"
	"github.com/OpenViCloud/incus-worker/incus/provider"
	"github.com/OpenViCloud/incus-worker/loop/command"
	"github.com/OpenViCloud/incus-worker/resource/volume"
)

// createPayload mirrors ProviderCommandQueue.EnqueueVolumeSnapshotCreateAsync.
type createPayload struct {
	SnapshotId       string `json:"snapshotId"`
	VolumeId         string `json:"volumeId"`
	ExternalVolumeId string `json:"externalVolumeId"`
	ProjectName      string `json:"projectName"`
	ZoneId           string `json:"zoneId"`
}

// Create runs volume-snapshot.create. Incus has made it once the operation returns, so it reports Created with the
// provider id "<volume>/<snapshot>": the create is done.
type Create struct {
	cp    *controlplane.Client
	zones cluster.Zones
}

func NewCreate(cp *controlplane.Client, zones cluster.Zones) *Create {
	return &Create{cp: cp, zones: zones}
}

func (h *Create) Run(ctx context.Context, leased command.Leased, logger *slog.Logger) error {
	var payload createPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	isComplete := payload.SnapshotId != "" && payload.VolumeId != "" && payload.ExternalVolumeId != "" && payload.ProjectName != ""
	if !isComplete {
		return command.Permanent(fmt.Errorf("volume-snapshot.create payload lacks snapshotId, volumeId, externalVolumeId or projectName"))
	}
	name, err := provider.SnapshotName(payload.SnapshotId)
	if err != nil {
		return command.Permanent(err)
	}
	zone, err := h.zones.Zone(ctx, leased.ZoneId)
	if err != nil {
		return err
	}
	pool := provider.StoragePoolOrDefault(zone.StoragePool)
	err = zone.Provider.CreateVolumeSnapshot(ctx, payload.ProjectName, pool, payload.ExternalVolumeId, name)
	// 409: an earlier run of this command made it (lab-storage 03); the name is ours.
	isOurs := err != nil && provider.IsConflict(err)
	if err != nil && !isOurs {
		return err
	}
	externalId := provider.SnapshotId(payload.ExternalVolumeId, name)
	if isOurs {
		logger.Info("already exists; treating it as ours", "object", externalId)
	}
	report := volume.StatusReport{Report: volume.ReportCreated, ExternalId: externalId}
	return volume.SendSnapshotStatus(ctx, h.cp, payload.VolumeId, payload.SnapshotId, report)
}

// GiveUp reports the request failed, with the provider id when the snapshot exists: then the create did happen.
func (h *Create) GiveUp(ctx context.Context, leased command.Leased, cause error, logger *slog.Logger) error {
	var payload createPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	report := volume.StatusReport{Report: volume.ReportRequestFailed, Reason: reasonCreateFailed, Detail: cause.Error()}
	report.ExternalId = h.existingSnapshot(ctx, leased.ZoneId, payload)
	return volume.SendSnapshotStatus(ctx, h.cp, payload.VolumeId, payload.SnapshotId, report)
}

func (h *Create) existingSnapshot(ctx context.Context, zoneId string, payload createPayload) string {
	name, err := provider.SnapshotName(payload.SnapshotId)
	if err != nil || payload.ProjectName == "" || payload.ExternalVolumeId == "" {
		return ""
	}
	zone, err := h.zones.Zone(ctx, zoneId)
	if err != nil {
		return ""
	}
	pool := provider.StoragePoolOrDefault(zone.StoragePool)
	snapshots, err := zone.Provider.ListVolumeSnapshots(ctx, payload.ProjectName, pool, payload.ExternalVolumeId)
	if err != nil {
		return ""
	}
	if !volume.SnapshotNames(payload.ExternalVolumeId, snapshots)[name] {
		return ""
	}
	return provider.SnapshotId(payload.ExternalVolumeId, name)
}
