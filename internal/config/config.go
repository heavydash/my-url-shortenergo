package config

import (
	"flag"
	"os"
)

type Config struct {
	ServerAddr      string
	BaseURL         string
	FileStoragePath string
}

func NewConfig() (*Config, error) {

	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	a := fs.String("a", ":8080", "address to run HTTP server")
	b := fs.String("b", "http://localhost:8080", "base URL for shortened links")
	f := fs.String("f", "", "file path to store the URL")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerAddr:      *a,
		BaseURL:         *b,
		FileStoragePath: *f,
	}

	if addr, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		cfg.ServerAddr = addr
	}
	if base, ok := os.LookupEnv("BASE_URL"); ok {
		cfg.BaseURL = base
	}
	if path, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.FileStoragePath = path
	}
	return cfg, nil
}
