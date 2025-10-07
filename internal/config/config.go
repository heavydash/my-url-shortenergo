package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	ServerAddr string
	BaseURL    string
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		ServerAddr: "localhost:8080",
		BaseURL:    "https://localhost:8080",
	}
	flag.StringVar(&cfg.ServerAddr, "a", cfg.ServerAddr, "address to run HTTP server")
	flag.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")
	flag.Parse()

	if addr, exists := os.LookupEnv("SERVER_ADDR"); exists && addr != "" {
		cfg.ServerAddr = addr
	}
	if url, exists := os.LookupEnv("BASE_URL"); exists && url != "" {
		cfg.BaseURL = url
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("https://%s/", cfg.ServerAddr)
	}
	return cfg, nil
}
