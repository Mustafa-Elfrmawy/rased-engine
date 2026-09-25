package pipeline

import (
	"log"

	"github.com/Mustafa-Elfrmawy/rased-engine/internal/models"
)

// PositionHandler defines a processing step in the pipeline.
// Each handler receives a Position and must call next() to pass it
// to the subsequent handler, or skip the chain by not calling next().
//
// Pipeline order: FilterHandler -> GeofenceHandler -> PublishHandler
type PositionHandler interface {
	Handle(position *models.Position, next func())
}

// Pipeline chains PositionHandler steps together. The pipeline is invoked
// once per decoded Position, and each handler may pass the position onward
// (by calling next()) or terminate the chain (by not calling next()).
//
// Mirrors Traccar's ProcessingHandler chain:
//
//	FilterHandler -> GeofenceHandler -> PublishHandler
type Pipeline struct {
	handlers []PositionHandler
}

// NewPipeline builds a pipeline performing, in order, the work of each
// provided handler.
func NewPipeline(handlers ...PositionHandler) *Pipeline {
	return &Pipeline{
		handlers: handlers,
	}
}

// Process runs the Position through the handler chain.
func (p *Pipeline) Process(position *models.Position) {
	if len(p.handlers) == 0 {
		return
	}
	p.run(0, position)
}

// run recursively invokes handlers in order.
func (p *Pipeline) run(index int, position *models.Position) {
	if index >= len(p.handlers) {
		return
	}
	handler := p.handlers[index]

	log.Printf("[Pipeline] Running handler %T for device %s", handler, position.DeviceID)

	handler.Handle(position, func() {
		p.run(index+1, position)
	})
}