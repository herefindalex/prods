package publishing

import (
	"context"
	"testing"
)

func TestEngineBeginDrainStopsNewPublicationDispatch(t *testing.T) {
	engine := &Engine{wake: make(chan struct{}, 1)}
	engine.BeginDrain()
	engine.Wake()
	if len(engine.wake) != 0 {
		t.Fatal("draining publisher accepted a new wake-up")
	}
	processed, err := engine.ProcessOne(t.Context())
	if err != nil || processed {
		t.Fatalf("draining publisher processed=%t err=%v", processed, err)
	}
}

// Existing repositories need not implement profile work. This optional adapter
// proves that profile-only progress keeps the serial publishing loop alive.
type profileOnlyRepository struct {
	Repository
	calls int
}

func (r *profileOnlyRepository) ProcessSpecNumericProfileWork(_ context.Context, limit int) (bool, error) {
	r.calls++
	return limit == 100, nil
}
func (r *profileOnlyRepository) ClaimPublicationIntent(context.Context) (Intent, bool, error) {
	return Intent{}, false, nil
}
func TestEngineProcessOneReportsProfileOnlyWork(t *testing.T) {
	repository := &profileOnlyRepository{}
	engine := &Engine{repository: repository}
	worked, err := engine.ProcessOne(t.Context())
	if err != nil || !worked || repository.calls != 1 {
		t.Fatalf("worked=%v calls=%d err=%v", worked, repository.calls, err)
	}
}
