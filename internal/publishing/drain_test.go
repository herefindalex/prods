package publishing

import "testing"

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
