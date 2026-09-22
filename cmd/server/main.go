package main

import (
	"log"
	"go-tracker-service/internal/config"
	
	"go-tracker-service/internal/core"
	"go-tracker-service/internal/pipeline"
	"go-tracker-service/internal/protocols/gt06n_tcp"
	"go-tracker-service/internal/redis"
)

func main() {
	log.Println("Starting GT06N Go Tracker Service...")

	cfg := config.LoadConfig()

	// Initialize the message broker client (Redis)
	redisClient := redis.NewClient(cfg)

	// Build the middleware pipeline:
	//   FilterHandler -> GeofenceHandler -> PublishHandler
	chain := pipeline.NewPipeline(
		pipeline.NewFilterHandler(),
		pipeline.NewGeofenceHandler(),
		pipeline.NewPublishHandler(redisClient),
	)

	// GT06N protocol handler (hardcoded for GT06N on port 5023)
	handler := gt06n_tcp.NewHandler(cfg.TCPPort, chain)

	// Start the GT06N TCP ingestion server (a goroutine per connection)
	server := core.NewServer(handler)
	server.Start(":" + cfg.TCPPort)
}
