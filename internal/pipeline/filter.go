package pipeline

import (
	"log"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/models"
)

type FilterHandler struct{}

func NewFilterHandler() *FilterHandler {
	return &FilterHandler{}
}

func (h *FilterHandler) Handle(position *models.Position, next func()) {
	if position.Latitude == 0 && position.Longitude == 0 {
		log.Printf("[Filter] Dropping position with zero coordinates for device %s", position.DeviceID)
		return
	}

	// Drop positions with unreasonable coordinate ranges
	if position.Latitude < -90 || position.Latitude > 90 ||
		position.Longitude < -180 || position.Longitude > 180 {
		log.Printf("[Filter] Dropping position with out-of-range coordinates for device %s: lat=%.6f lon=%.6f",
			position.DeviceID, position.Latitude, position.Longitude)
		return
	}
	log.Printf("[Filter] Position passed for device %s", position.DeviceID)
	next()
}
