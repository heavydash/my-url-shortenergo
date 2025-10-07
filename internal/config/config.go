package config

import (
	"flag"
	"fmt"
)

type Config struct {
	ServerAddr string
	BaseURL    string
}

func NewConfig() (*Config, error) {
	cfg := &Config{}
	flag.StringVar(&cfg.ServerAddr, "a", "localhost:8080", "address to run HTTP server")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080/", "base URL for shortened links")
	flag.Parse()

	if cfg.ServerAddr == "" {
		return nil, fmt.Errorf("server address (-a) is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("https://%s/", cfg.ServerAddr)
	}
	return cfg, nil
}
