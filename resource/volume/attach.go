package volume

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/lxc/incus/v7/shared/api"

	"github.com/OpenViCloud/incus-worker/controlplane"
	"github.com/OpenViCloud/incus-worker/incus/cluster"
	"github.com/OpenViCloud/incus-worker/incus/provider"
	"github.com/OpenViCloud/incus-worker/loop/command"
)

// attachPayload mirrors EnqueueVolumeAttachAsync and EnqueueVolumeDetachAsync (detach has no externalVmId).
type attachPayload struct {
	VolumeId         string `json:"volumeId"`
	ExternalVolumeId string `json:"externalVolumeId"`
	ExternalVmId     string `json:"externalVmId"`
	ProjectName      string `json:"projectName"`
	ZoneId           string `json:"zoneId"`
}

// Attach runs volume.attach: a disk device named after the volume, hot-plugged by PATCH (lab-storage 04: works on a
// running VM, and the same PATCH again is a no-op). It reports what Incus then shows: the instances using the volume.
type Attach struct {
	cp    *controlplane.Client
	zones cluster.Zones
}

func NewAttach(cp *controlplane.Client, zones cluster.Zones) *Attach {
	return &Attach{cp: cp, zones: zones}
}

func (h *Attach) Run(ctx context.Context, leased command.Leased, logger *slog.Logger) error {
	var payload attachPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	isComplete := payload.VolumeId != "" && payload.ExternalVolumeId != "" && payload.ExternalVmId != "" && payload.ProjectName != ""
	if !isComplete {
		return command.Permanent(fmt.Errorf("volume.attach payload lacks volumeId, externalVolumeId, externalVmId or projectName"))
	}
	zone, err := h.zones.Zone(ctx, leased.ZoneId)
	if err != nil {
		return err
	}
	device := map[string]string{
		"type":   "disk",
		"pool":   provider.StoragePoolOrDefault(zone.StoragePool),
		"source": payload.ExternalVolumeId,
	}
	patch := api.InstancePut{Devices: map[string]map[string]string{payload.ExternalVolumeId: device}}
	err = zone.Provider.PatchInstance(ctx, payload.ProjectName, payload.ExternalVmId, patch)
	if provider.IsNotFound(err) {
		// The VM is gone: no later lease can attach to it.
		return command.Permanent(err)
	}
	if err != nil {
		return err
	}
	return reportWhatIncusShows(ctx, h.cp, zone.Provider, payload, provider.StoragePoolOrDefault(zone.StoragePool))
}

// GiveUp reports the request failed, carrying what Incus shows: an attach that happened while its report failed is then
// fulfilled by it, and only one that did not is withdrawn.
func (h *Attach) GiveUp(ctx context.Context, leased command.Leased, cause error, logger *slog.Logger) error {
	return giveUp(ctx, h.cp, h.zones, leased, cause, reasonAttachFailed)
}

// reportWhatIncusShows reads the volume after an attach or detach and reports it.
func reportWhatIncusShows(ctx context.Context, cp *controlplane.Client, p provider.Provider, payload attachPayload, pool string) error {
	volume, err := p.GetVolume(ctx, payload.ProjectName, pool, payload.ExternalVolumeId)
	if err != nil {
		return err
	}
	return SendStatus(ctx, cp, payload.VolumeId, Observed(true, InstanceNames(volume.UsedBy)))
}

// giveUp is the attach's and the detach's: the request failed, plus what Incus shows when it can be read — the volume
// gone, or the instances using it. Unreadable, the report says only that the request failed.
func giveUp(ctx context.Context, cp *controlplane.Client, zones cluster.Zones, leased command.Leased, cause error, reason string) error {
	var payload attachPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	report := StatusReport{Report: ReportRequestFailed, Reason: reason, Detail: cause.Error()}
	observation, read := readAttachment(ctx, zones, leased.ZoneId, payload)
	if read {
		report.Exists = observation.Exists
		report.AttachedTo = observation.AttachedTo
	}
	return SendStatus(ctx, cp, payload.VolumeId, report)
}

// readAttachment reads whether the volume exists and the instances using it; false when it cannot be read.
func readAttachment(ctx context.Context, zones cluster.Zones, zoneId string, payload attachPayload) (StatusReport, bool) {
	if payload.ProjectName == "" || payload.ExternalVolumeId == "" {
		return StatusReport{}, false
	}
	zone, err := zones.Zone(ctx, zoneId)
	if err != nil {
		return StatusReport{}, false
	}
	pool := provider.StoragePoolOrDefault(zone.StoragePool)
	volume, err := zone.Provider.GetVolume(ctx, payload.ProjectName, pool, payload.ExternalVolumeId)
	if provider.IsNotFound(err) {
		return Observed(false, nil), true
	}
	if err != nil {
		return StatusReport{}, false
	}
	return Observed(true, InstanceNames(volume.UsedBy)), true
}

// InstanceNames is the instance names of a used_by, in order.
func InstanceNames(usedBy []string) []string {
	names := []string{}
	for _, instance := range InstancesUsing(usedBy) {
		names = append(names, instance.Name)
	}
	return names
}

// InstanceRef is one instance named in a volume's used_by.
type InstanceRef struct {
	Project string
	Name    string
}

// InstancesUsing reads used_by ("/1.0/instances/<name>?project=<project>", lab-storage 04). Entries for anything
// else (a snapshot, a profile) are not instances a detach can edit.
func InstancesUsing(usedBy []string) []InstanceRef {
	instances := []InstanceRef{}
	for _, entry := range usedBy {
		parsed, err := url.Parse(entry)
		if err != nil {
			continue
		}
		name, isInstance := strings.CutPrefix(parsed.Path, "/1.0/instances/")
		if !isInstance || name == "" || strings.Contains(name, "/") {
			continue
		}
		project := parsed.Query().Get("project")
		if project == "" {
			project = "default"
		}
		instances = append(instances, InstanceRef{Project: project, Name: name})
	}
	return instances
}
