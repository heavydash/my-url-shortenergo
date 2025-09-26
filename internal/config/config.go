package config

import (
	"flag"
	"log"
)

type Config struct {
	ServerAddr string
	BaseURL    string
}

func NewConfig() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.ServerAddr, "a", "localhost:8080", "address to run HTTP server")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080/", "base URL for shortened links")

	flag.Parse()

	if cfg.ServerAddr == "" {
		log.Fatal("Server address (-a) is required")
	}
	if cfg.BaseURL == "" {
		log.Fatal("Base URL (-b) is required")
	}

	return cfg
}
