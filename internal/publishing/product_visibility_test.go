package publishing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Keep real staging and admission while making a catalogue-wide read fail.
// A Product commit has enough information to read only that Product instead.
type productVisibilityRepository struct {
	*workerRefreshRepository
	readError error
}

func (r *productVisibilityRepository) ActivePublications(context.Context) ([]ActivePublication, error) {
	return nil, errors.New("unrelated catalogue must not be reloaded")
}

func (r *productVisibilityRepository) ActivePublication(ctx context.Context, id string) (ActivePublication, bool, error) {
	if r.readError != nil {
		return ActivePublication{}, false, r.readError
	}
	return r.workerRefreshRepository.ActivePublication(ctx, id)
}

func installTestProduct(ctx context.Context, coordinator VisibilityCoordinator, id string) error {
	if incremental, ok := coordinator.(interface {
		InstallProductVisibility(context.Context, string) error
	}); ok {
		return incremental.InstallProductVisibility(ctx, id)
	}
	return coordinator.InstallVisibility(ctx)
}

func (r *productVisibilityRepository) ActivatePublication(ctx context.Context, _ Intent, a ActivePublication, coordinator VisibilityCoordinator) (bool, error) {
	coordinator.LockVisibility()
	defer coordinator.UnlockVisibility()
	a.VisibilityGeneration = int64(len(r.active) + 10)
	r.active = append(r.active, a)
	return true, installTestProduct(ctx, coordinator, a.ProductID)
}

func TestProductPublicationDoesNotReloadUnchangedUnits(t *testing.T) {
	e, base := workerRefreshFixture(t)
	r := &productVisibilityRepository{workerRefreshRepository: base}
	e.repository = r
	old := r.active[0]
	// Previously admitted immutable units do not need a new manifest validation
	// just because another Product becomes public. Startup still validates all.
	if err := os.Remove(filepath.Join(e.root, "units", old.ArtifactID, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if worked, err := e.ProcessOne(t.Context()); err != nil || !worked {
		t.Fatalf("publish another Product: worked=%v err=%v", worked, err)
	}
	for _, route := range []string{old.Route, "/products/first"} {
		lease, err := e.AdmitPath(route)
		if err != nil {
			t.Fatalf("admit %s: %v", route, err)
		}
		lease.File.Close()
	}
}

func TestProductVisibilityReadFailureFailsClosed(t *testing.T) {
	e, base := workerRefreshFixture(t)
	e.repository = &productVisibilityRepository{workerRefreshRepository: base, readError: errors.New("committed Product cannot be read")}
	e.LockVisibility()
	err := installTestProduct(t.Context(), e, base.active[0].ProductID)
	e.UnlockVisibility()
	if err == nil {
		t.Fatal("expected installation failure")
	}
	if _, err := e.AdmitPath(base.active[0].Route); !errors.Is(err, ErrPublicUnavailable) {
		t.Fatalf("old visibility was admitted after installation failed: %v", err)
	}
}
