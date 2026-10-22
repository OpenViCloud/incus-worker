package vmsnapshot

import (
	"context"
	"log/slog"

	"github.com/lxc/incus/v7/shared/api"

	"github.com/OpenViCloud/titan-incus-worker/controlplane"
	"github.com/OpenViCloud/titan-incus-worker/incus/cluster"
	"github.com/OpenViCloud/titan-incus-worker/incus/provider"
	"github.com/OpenViCloud/titan-incus-worker/loop/command"
)

// Revert runs vm-snapshot.revert: the instance's disk is put back to the snapshot, running or not (lab-storage 05).
// Disk only: a full restore also puts back the snapshot's devices and config, which would attach or detach volumes
// and resize the VM behind Titan's back. It reports Reverted, which ends the revert; giving up reports the request failed.
type Revert struct {
	cp    *controlplane.Client
	zones cluster.Zones
}

func NewRevert(cp *controlplane.Client, zones cluster.Zones) *Revert {
	return &Revert{cp: cp, zones: zones}
}

func (h *Revert) Run(ctx context.Context, leased command.Leased, logger *slog.Logger) error {
	payload, instance, snapshot, err := decodeSnapshot(leased)
	if err != nil {
		return err
	}
	zone, err := h.zones.Zone(ctx, leased.ZoneId)
	if err != nil {
		return err
	}
	err = zone.Provider.UpdateInstance(ctx, payload.ProjectName, instance, api.InstancePut{Restore: snapshot, DiskOnly: true})
	if provider.IsNotFound(err) {
		// The VM is gone: no later lease can revert it.
		return command.Permanent(err)
	}
	if err != nil {
		return err
	}
	return sendStatus(ctx, h.cp, payload.VmId, payload.SnapshotId, statusReport{Report: reportReverted})
}

func (h *Revert) GiveUp(ctx context.Context, leased command.Leased, cause error, logger *slog.Logger) error {
	var payload snapshotPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	return sendStatus(ctx, h.cp, payload.VmId, payload.SnapshotId, requestFailed(reasonRevertFailed, cause))
}
