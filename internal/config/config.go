package config

import "os"

type Config struct {
	// GT06N TCP listener port
	TCPPort string

	// Redis message broker settings
	RedisHost     string
	RedisPort     string
	RedisPassword string
}

func LoadConfig() Config {
	return Config{
		// GT06N listens on port 5023 (Layer 1: Connection-Based Routing)
		TCPPort:       getEnv("TCP_PORT", "5023"),
		RedisHost:     getEnv("REDIS_HOST", "redis_GpsTrakerSystemManagment"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
