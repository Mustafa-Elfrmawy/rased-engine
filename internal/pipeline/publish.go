package pipeline

import (
	"encoding/json"
	"log"

	"github.com/Mustafa-Elfrmawy/rased-engine/internal/models"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/redis"
)

// PublishHandler is the final stage of the pipeline.
// It serializes the enriched Position to JSON and pushes it to the
// message broker (Redis List) for downstream consumption by the
// Laravel Queue worker.
type PublishHandler struct {
	redisClient *redis.Client
}

func NewPublishHandler(redisClient *redis.Client) *PublishHandler {
	return &PublishHandler{
		redisClient: redisClient,
	}
}

func (h *PublishHandler) Handle(position *models.Position, next func()) {
	jsonBytes, err := json.Marshal(position)
	if err != nil {
		log.Printf("[Publish] Failed to marshal position for device %s: %v", position.DeviceID, err)
		return
	}

	// Simulate broker publish — in production this pushes to Redis List
	// which the Laravel Queue worker consumes for MySQL insertion.
	//
	// For now, we also log the JSON so it's visible in stdout during development.
	log.Printf("[Publish] Position JSON for device %s: %s", position.DeviceID, string(jsonBytes))

	if h.redisClient != nil {
		if err := h.redisClient.PublishLocation(position); err != nil {
			log.Printf("[Publish] Failed to publish to Redis for device %s: %v", position.DeviceID, err)
		}
	}

	next()
}
