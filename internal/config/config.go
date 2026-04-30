package config

import "os"

type Config struct {
	DatabaseURL string
	APIPort     string
	APIKey      string
}

func Load() Config {
	return Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIPort:     os.Getenv("API_PORT"),
		APIKey:      os.Getenv("API_KEY"),
	}
}
