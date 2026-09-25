package main

import (
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/config"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/core"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/pipeline"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/protocols/gt06n_tcp"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/redis"
)

func main() {



	cfg := config.LoadConfig()

	redisClient := redis.NewClient(cfg)


	// Build the middleware pipeline:
	//   FilterHandler -> GeofenceHandler -> PublishHandler
	chain := pipeline.NewPipeline(
		pipeline.NewFilterHandler(),
		pipeline.NewGeofenceHandler(),
		pipeline.NewPublishHandler(redisClient),
	)
	// chain is a Pipeline struct that holds a slice of handlers,
	//  where each handler implements the PositionHandler interface.


	// GT06N protocol handler (hardcoded for GT06N on port 5023)
	handler := gt06n_tcp.NewHandler(cfg.TCPPort, chain)

	server := core.NewServer(handler)
	server.Start(":" + cfg.TCPPort)
}
