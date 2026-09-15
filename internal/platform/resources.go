package platform

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
)

var (
	ErrResourceUnavailable = errors.New("resource capacity cannot be inspected")
	ErrResourceCritical    = errors.New("resource gate rejected operation")
)

type ResourceStats struct {
	VolumeID       string
	TotalBytes     uint64
	FreeBytes      uint64
	FreeInodes     uint64
	SupportsInodes bool
}

type ResourceRequest struct {
	Operation      string
	Path           string
	RequiredBytes  uint64
	ByteHeadroom   uint64
	MinFreePercent uint8
	RequiredInodes uint64
	InodeHeadroom  uint64
}

type ResourceSnapshot struct {
	ResourceStats
	ReservedBytes  uint64
	ReservedInodes uint64
}

type ResourceProbe func(path string) (ResourceStats, error)

type ResourceGate struct {
	probe ResourceProbe
	mu    sync.Mutex
	byVol map[string]resourceReservation
}

type resourceReservation struct {
	bytes  uint64
	inodes uint64
}

type Reservation struct {
	gate      *ResourceGate
	volumeID  string
	bytes     uint64
	inodes    uint64
	releaseMu sync.Once
}

func NewResourceGate(probe ResourceProbe) *ResourceGate {
	if probe == nil {
		probe = probeResourceVolume
	}
	return &ResourceGate{probe: probe, byVol: make(map[string]resourceReservation)}
}

func (g *ResourceGate) Inspect(ctx context.Context, path string) (ResourceSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ResourceSnapshot{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ResourceSnapshot{}, fmt.Errorf("%w: resource path is empty", ErrResourceUnavailable)
	}
	stats, err := g.probe(path)
	if err != nil {
		return ResourceSnapshot{}, fmt.Errorf("%w at %s: %v", ErrResourceUnavailable, path, err)
	}
	stats.VolumeID = strings.TrimSpace(stats.VolumeID)
	if stats.VolumeID == "" {
		return ResourceSnapshot{}, fmt.Errorf("%w: volume identity is empty", ErrResourceUnavailable)
	}
	g.mu.Lock()
	reserved := g.byVol[stats.VolumeID]
	g.mu.Unlock()
	return ResourceSnapshot{ResourceStats: stats, ReservedBytes: reserved.bytes, ReservedInodes: reserved.inodes}, nil
}

func (g *ResourceGate) Admit(ctx context.Context, request ResourceRequest) (*Reservation, ResourceSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, ResourceSnapshot{}, err
	}
	request.Operation = strings.TrimSpace(request.Operation)
	request.Path = strings.TrimSpace(request.Path)
	if request.Operation == "" || request.Path == "" || request.MinFreePercent > 100 || addOverflows(request.RequiredBytes, request.ByteHeadroom) ||
		addOverflows(request.RequiredInodes, request.InodeHeadroom) {
		return nil, ResourceSnapshot{}, fmt.Errorf("%w: invalid resource request", ErrResourceCritical)
	}
	stats, err := g.probe(request.Path)
	if err != nil {
		return nil, ResourceSnapshot{}, fmt.Errorf("%w for %s at %s: %v", ErrResourceUnavailable, request.Operation, request.Path, err)
	}
	stats.VolumeID = strings.TrimSpace(stats.VolumeID)
	if stats.VolumeID == "" {
		return nil, ResourceSnapshot{}, fmt.Errorf("%w for %s: volume identity is empty", ErrResourceUnavailable, request.Operation)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	reserved := g.byVol[stats.VolumeID]
	snapshot := ResourceSnapshot{ResourceStats: stats, ReservedBytes: reserved.bytes, ReservedInodes: reserved.inodes}
	requiredHeadroom := request.ByteHeadroom
	if request.MinFreePercent > 0 {
		if stats.TotalBytes == 0 {
			return nil, snapshot, fmt.Errorf("%w for %s at %s: total capacity is unavailable", ErrResourceUnavailable, request.Operation, request.Path)
		}
		if percentHeadroom := percentageCeiling(stats.TotalBytes, request.MinFreePercent); percentHeadroom > requiredHeadroom {
			requiredHeadroom = percentHeadroom
		}
	}
	if reserved.bytes > stats.FreeBytes || request.RequiredBytes > stats.FreeBytes-reserved.bytes ||
		requiredHeadroom > stats.FreeBytes-reserved.bytes-request.RequiredBytes {
		return nil, snapshot, fmt.Errorf(
			"%w: %s needs %d bytes plus %d headroom at %s; free=%d reserved=%d",
			ErrResourceCritical, request.Operation, request.RequiredBytes, request.ByteHeadroom, request.Path, stats.FreeBytes, reserved.bytes,
		)
	}
	if stats.SupportsInodes {
		neededInodes := request.RequiredInodes + request.InodeHeadroom
		if reserved.inodes > stats.FreeInodes || neededInodes > stats.FreeInodes-reserved.inodes {
			return nil, snapshot, fmt.Errorf(
				"%w: %s needs %d inodes plus %d headroom at %s; free=%d reserved=%d",
				ErrResourceCritical, request.Operation, request.RequiredInodes, request.InodeHeadroom, request.Path, stats.FreeInodes, reserved.inodes,
			)
		}
	}
	reserved.bytes += request.RequiredBytes
	reserved.inodes += request.RequiredInodes
	g.byVol[stats.VolumeID] = reserved
	return &Reservation{
		gate: g, volumeID: stats.VolumeID, bytes: request.RequiredBytes, inodes: request.RequiredInodes,
	}, snapshot, nil
}

func (reservation *Reservation) Release() {
	if reservation == nil || reservation.gate == nil {
		return
	}
	reservation.releaseMu.Do(func() {
		gate := reservation.gate
		gate.mu.Lock()
		defer gate.mu.Unlock()
		current := gate.byVol[reservation.volumeID]
		if current.bytes >= reservation.bytes {
			current.bytes -= reservation.bytes
		} else {
			current.bytes = 0
		}
		if current.inodes >= reservation.inodes {
			current.inodes -= reservation.inodes
		} else {
			current.inodes = 0
		}
		if current.bytes == 0 && current.inodes == 0 {
			delete(gate.byVol, reservation.volumeID)
		} else {
			gate.byVol[reservation.volumeID] = current
		}
	})
}

func addOverflows(left, right uint64) bool {
	return left > math.MaxUint64-right
}

func percentageCeiling(total uint64, percent uint8) uint64 {
	whole := total / 100 * uint64(percent)
	remainder := total % 100 * uint64(percent)
	return whole + (remainder+99)/100
}
