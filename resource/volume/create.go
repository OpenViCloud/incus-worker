package volume

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/lxc/incus/v7/shared/api"

	"github.com/OpenViCloud/incus-worker/controlplane"
	"github.com/OpenViCloud/incus-worker/incus/cluster"
	"github.com/OpenViCloud/incus-worker/incus/provider"
	"github.com/OpenViCloud/incus-worker/loop/command"
)

// DisplayNameKey carries the name the tenant typed.
const DisplayNameKey = "user.titan-name"

// createPayload mirrors ProviderCommandQueue.EnqueueVolumeCreateAsync.
type createPayload struct {
	VolumeId    string `json:"volumeId"`
	Name        string `json:"name"`
	ZoneId      string `json:"zoneId"`
	SizeGb      int    `json:"sizeGb"`
	ProjectName string `json:"projectName"`
}

// Create runs volume.create: an empty block volume in the tenant's project. Incus has made it once the request
// returns, so it reports Created with the provider id: the create is done.
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
	isComplete := payload.VolumeId != "" && payload.ProjectName != "" && payload.SizeGb > 0
	if !isComplete {
		return command.Permanent(fmt.Errorf("volume.create payload lacks volumeId, projectName or sizeGb"))
	}
	name, err := provider.VolumeName(payload.VolumeId)
	if err != nil {
		return command.Permanent(err)
	}
	zone, err := h.zones.Zone(ctx, leased.ZoneId)
	if err != nil {
		return err
	}
	if err := provider.EnsureProject(ctx, zone.Provider, payload.ProjectName); err != nil {
		return err
	}

	request := api.StorageVolumesPost{
		Name:        name,
		Type:        "custom",
		ContentType: "block",
		StorageVolumePut: api.StorageVolumePut{Config: map[string]string{
			"size":         strconv.Itoa(payload.SizeGb) + "GiB",
			DisplayNameKey: payload.Name,
		}},
	}
	err = zone.Provider.CreateVolume(ctx, payload.ProjectName, provider.StoragePoolOrDefault(zone.StoragePool), request)
	// 409: an earlier run of this command made it (lab-storage 02); the name is ours.
	isOurs := err != nil && provider.IsConflict(err)
	if err != nil && !isOurs {
		return err
	}
	if isOurs {
		logger.Info("already exists; treating it as ours", "object", name)
	}
	return SendStatus(ctx, h.cp, payload.VolumeId, StatusReport{Report: ReportCreated, ExternalId: name})
}

// GiveUp reports the request failed, with the provider id when the volume exists: then the create did happen.
func (h *Create) GiveUp(ctx context.Context, leased command.Leased, cause error, logger *slog.Logger) error {
	var payload createPayload
	if err := command.DecodePayload(leased, &payload); err != nil {
		return err
	}
	report := StatusReport{Report: ReportRequestFailed, Reason: ReasonCreateFailed, Detail: cause.Error()}
	report.ExternalId = ExistingVolume(ctx, h.zones, leased.ZoneId, payload.ProjectName, payload.VolumeId)
	return SendStatus(ctx, h.cp, payload.VolumeId, report)
}

// ExistingVolume returns the volume's Incus name when it is there, "" when it is not or cannot be read.
func ExistingVolume(ctx context.Context, zones cluster.Zones, zoneId string, project string, volumeId string) string {
	if project == "" {
		return ""
	}
	name, err := provider.VolumeName(volumeId)
	if err != nil {
		return ""
	}
	zone, err := zones.Zone(ctx, zoneId)
	if err != nil {
		return ""
	}
	_, err = zone.Provider.GetVolume(ctx, project, provider.StoragePoolOrDefault(zone.StoragePool), name)
	if err != nil {
		return ""
	}
	return name
}
