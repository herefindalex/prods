package publishing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
)

func NormalizeSpecSearchBackend(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "bleve", nil
	}
	if value != "bleve" && value != "sqlite" {
		return "", fmt.Errorf("invalid spec search backend %q: expected bleve or sqlite", value)
	}
	return value, nil
}

type SpecSearchDiagnostics struct {
	Backend          string
	Ready            bool
	SourceDigest     string
	LastError        string
	CandidateQueries uint64
	FallbackQueries  uint64
}

type specSearchStatus struct {
	digest    string
	lastError string
}

func (e *Engine) SpecSearchDiagnostics() SpecSearchDiagnostics {
	d := SpecSearchDiagnostics{Backend: e.specBackend, CandidateQueries: e.specCandidateQueries.Load(), FallbackQueries: e.specFallbackQueries.Load()}
	// Observe the real adapter digest before taking the public gate. Replacement
	// may wait on disk/ownership, and must never extend request admission locks.
	if e.specBleve != nil {
		d.SourceDigest = e.specBleve.digest()
	}
	// Atomic status never waits for adapter IO while holding the public gate.
	e.gate.RLock()
	defer e.gate.RUnlock()
	status := e.specStatus.Load()
	if status != nil {
		if e.specBackend != "bleve" {
			d.SourceDigest = status.digest
		}
		d.LastError = status.lastError
	}
	d.Ready = !e.draining.Load() && e.index != nil && e.index.unhealthy == nil && e.index.specDigestError == nil && status != nil && status.lastError == "" && d.SourceDigest != "" && d.SourceDigest == e.index.specDigest && status.digest == e.index.specDigest
	return d
}

func (e *Engine) specSearchFailure(err error) {
	if err == nil {
		return
	}
	e.specStatus.Store(&specSearchStatus{lastError: err.Error()})
	slog.Warn("spec search candidate index unavailable; using active public facts", "backend", e.specBackend, "error", err)
}

func (e *Engine) installSpecSearchLocked(index *publicIndex) {
	active := make([]ActivePublication, 0, len(index.byID))
	index.specInventory = make(map[string]SpecCandidateBinding, len(index.byID))
	for id, a := range index.byID {
		active = append(active, a)
		index.specInventory[id] = SpecCandidateBinding{ProductID: a.ProductID, PublicRevision: a.PublicRevision, SourceRevision: a.SourceRevision, SiteEpoch: a.SiteEpoch, VisibilityGeneration: a.VisibilityGeneration, ProjectionVersion: SpecProjectionVersion, SpecIDs: SpecProjectionIDs(a.View)}
	}
	index.specDigest, index.specDigestError = specActiveDigest(active)
	if index.specDigestError != nil {
		e.specStatus.Store(&specSearchStatus{lastError: index.specDigestError.Error()})
	}
	e.specDirty.Store(true)
	e.Wake()
}

func (e *Engine) RebuildSpecSearchIndex(ctx context.Context) error {
	return e.refreshSpecSearch(ctx, true)
}

// Refresh serializes derived writes only. Consume dirty BEFORE capture so an
// install overlapping IO leaves its own dirty flag and wake-up intact. No
// caller activation is used: visibility generations come from committed index.
func (e *Engine) refreshSpecSearch(ctx context.Context, force bool) error {
	e.specRefreshMu.Lock()
	defer e.specRefreshMu.Unlock()
	if !force && !e.specDirty.Swap(false) {
		return nil
	}
	if force {
		e.specDirty.Store(false)
	}
	e.gate.RLock()
	index := e.index
	var active []ActivePublication
	var digest string
	var err error
	if index == nil || index.unhealthy != nil {
		err = ErrPublicUnavailable
	} else {
		digest, err = index.specDigest, index.specDigestError
		active = make([]ActivePublication, 0, len(index.byID))
		for _, a := range index.byID {
			active = append(active, a)
		}
	}
	e.gate.RUnlock()
	if err == nil && digest == "" {
		digest, err = specActiveDigest(active)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && e.draining.Load() {
		err = errors.New("spec search index closed")
	}
	if err == nil {
		switch e.specBackend {
		case "bleve":
			if e.specBleve == nil {
				err = errors.New("Bleve spec search adapter unavailable")
				if status := e.specStatus.Load(); status != nil && status.lastError != "" {
					err = errors.New(status.lastError)
				}
			} else if force {
				err = e.specBleve.rebuild(ctx, active)
			} else {
				err = e.specBleve.replace(ctx, active)
			}
		case "sqlite":
			repository, ok := e.repository.(SpecSearchRepository)
			if !ok {
				err = errors.New("SQLite spec search repository unavailable")
			} else {
				err = repository.RebuildSpecSearchProjection(ctx, active)
				if err == nil {
					var snapshot SpecCandidateSnapshot
					snapshot, err = repository.SpecSearchCandidates(ctx, nil, SpecProjectionVersion)
					if err == nil && !specInventoryMatches(snapshot, index.specInventory) {
						err = errors.New("SQLite spec search inventory mismatch")
					}
				}
			}
		default:
			return nil // Legacy directly constructed Engine fixtures.
		}
	}
	if err != nil {
		e.specSearchFailure(err)
		// Retry on a later worker tick; avoid a busy loop on persistent failure.
		if !e.draining.Load() {
			e.specDirty.Store(true)
		}
		return err
	}
	e.gate.Lock()
	if e.index != index {
		// Aggregate consolidation can replace the pointer without changing
		// authority or setting dirty. Retry the deferred digest on that snapshot.
		e.specDirty.Store(true)
	} else if index.specDigest == "" {
		next := *index
		next.specDigest = digest
		e.index = &next
	}
	e.gate.Unlock()
	e.specStatus.Store(&specSearchStatus{digest: digest})
	return nil
}

func specInventoryMatches(snapshot SpecCandidateSnapshot, expected map[string]SpecCandidateBinding) bool {
	if snapshot.ProjectionVersion != SpecProjectionVersion || len(snapshot.Inventory) != len(expected) {
		return false
	}
	seen := make(map[string]bool, len(expected))
	for _, binding := range snapshot.Inventory {
		want, ok := expected[binding.ProductID]
		ids := slices.Clone(binding.SpecIDs)
		sort.Strings(ids)
		if !ok || seen[binding.ProductID] || !specCachedBindingMatches(binding, want) || !slices.Equal(ids, want.SpecIDs) {
			return false
		}
		seen[binding.ProductID] = true
	}
	return true
}

func specCachedBindingMatches(binding, expected SpecCandidateBinding) bool {
	return binding.ProjectionVersion == expected.ProjectionVersion && binding.ProductID == expected.ProductID && binding.PublicRevision == expected.PublicRevision && binding.SourceRevision == expected.SourceRevision && binding.SiteEpoch == expected.SiteEpoch && binding.VisibilityGeneration == expected.VisibilityGeneration
}
