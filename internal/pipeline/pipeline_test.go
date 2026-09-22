package pipeline

import (
	"testing"

	"go-tracker-service/internal/models"
)

// recordingHandler records invocation order for chain verification.
type recordingHandler struct {
	name  string
	order *[]string
}

func (r *recordingHandler) Handle(position *models.Position, next func()) {
	*r.order = append(*r.order, r.name)
	next()
}

type dropHandler struct{}

func (d *dropHandler) Handle(position *models.Position, next func()) {
	// Simulates FilterHandler rejecting the position
}

func TestPipelineChainOrder(t *testing.T) {
	var order []string
	p := NewPipeline(
		&recordingHandler{name: "a", order: &order},
		&recordingHandler{name: "b", order: &order},
		&recordingHandler{name: "c", order: &order},
	)

	p.Process(&models.Position{DeviceID: "test"})

	if len(order) != 3 {
		t.Fatalf("expected 3 handler invocations, got %d: %v", len(order), order)
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("wrong invocation order: %v", order)
	}
}

func TestPipelineShortCircuit(t *testing.T) {
	var order []string
	p := NewPipeline(
		&dropHandler{},
		&recordingHandler{name: "never", order: &order},
	)

	p.Process(&models.Position{DeviceID: "test"})

	if len(order) != 0 {
		t.Errorf("handler chain should have been short-circuited, got: %v", order)
	}
}

func TestFilterHandlerDropsZeroCoords(t *testing.T) {
	f := NewFilterHandler()
	called := false
	f.Handle(&models.Position{DeviceID: "x"}, func() {
		called = true
	})
	if called {
		t.Errorf("filter should drop positions with zero coordinates")
	}

	called = false
	f.Handle(&models.Position{DeviceID: "x", Latitude: 30.0, Longitude: 31.0}, func() {
		called = true
	})
	if !called {
		t.Errorf("filter should pass valid coordinates")
	}
}

func TestGeofenceHandlerPassThrough(t *testing.T) {
	g := NewGeofenceHandler()
	called := false
	g.Handle(&models.Position{DeviceID: "x"}, func() {
		called = true
	})
	if !called {
		t.Errorf("geofence handler must be a pass-through for now")
	}
}

func TestPublishHandlerNilRedis(t *testing.T) {
	// A nil redis client must not panic — the JSON is still logged.
	p := NewPublishHandler(nil)
	called := false
	p.Handle(&models.Position{DeviceID: "x", Latitude: 30.0, Longitude: 31.0}, func() {
		called = true
	})
	if !called {
		t.Errorf("publish handler should continue the chain even with nil redis")
	}
}