package config

import (
	"log"
	"os"
)

type Config struct {
	// GT06N TCP listener port
	TCPPort string

	// Redis message broker settings
	RedisHost     string
	RedisPort     string
	RedisPassword string
}

func LoadConfig() Config {
	temp := []string{
	   "TCP_PORT",
	   "REDIS_HOST",
	   "REDIS_PORT",
	   "REDIS_PASSWORD",
	}
	for i := 0; i < 4; i++ {
		if err := getEnv(temp[i] , "nil"); err == "nil" {
		log.Printf("not exit in env os%v" , temp[i])
	}
}
	return Config{
		// GT06N listens on port 5023 (Layer 1: Connection-Based Routing)
		TCPPort:       getEnv("TCP_PORT", "5028"),
		RedisHost:     getEnv("REDIS_HOST", "redis_rased"),
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
