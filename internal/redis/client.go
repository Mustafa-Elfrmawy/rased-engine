package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"github.com/redis/go-redis/v9"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/config"
	"github.com/Mustafa-Elfrmawy/rased-engine/internal/models"
)

type Client struct {
	client *redis.Client
	ctx    context.Context
}

func NewClient(cfg config.Config) *Client {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       0,
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Warning: Failed to connect to Redis at %s:%s: %v", cfg.RedisHost, cfg.RedisPort, err)
	} else {
		log.Printf("Redis connection established at %s:%s", cfg.RedisHost, cfg.RedisPort)
	}

	return &Client{
		client: rdb,
		ctx:    ctx,
	}
}

// PublishLocation pushes the normalized GT06N position to the message broker.
//
// Design: The engine does NOT write directly to MySQL/PostgreSQL. Instead the
// enriched Position is serialized to JSON and pushed to a Redis List, which a
// separate Laravel Queue worker consumes for database insertion (decoupled
// persistence).
func (c *Client) PublishLocation(payload *models.Position) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Redis List acts as the queue for the Laravel worker
	err = c.client.LPush(c.ctx, "tracker:locations", jsonPayload).Err()
	if err != nil {
		return err
	}

	// Also store the latest location per device for quick retrieval
	return c.client.Set(c.ctx, fmt.Sprintf("device:location:%s", payload.DeviceID), jsonPayload, 0).Err()
}
