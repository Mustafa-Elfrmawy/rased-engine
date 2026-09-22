package pipeline

import (
	"log"

	"go-tracker-service/internal/models"
)

type GeofenceHandler struct{}

func NewGeofenceHandler() *GeofenceHandler {
	return &GeofenceHandler{}
}

func (h *GeofenceHandler) Handle(position *models.Position, next func()) {
	// TODO: Implement Tile38 integration later
	log.Printf("[Geofence] Pass-through for device %s", position.DeviceID)
	next()
}
