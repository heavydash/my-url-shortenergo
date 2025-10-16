package config

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	ServerAddr string
	BaseURL    string
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		ServerAddr: ":8080",
		BaseURL:    "http://localhost:8080",
	}

	fs := flag.NewFlagSet("my-url-shortener", flag.ContinueOnError)
	fs.StringVar(&cfg.ServerAddr, "a", cfg.ServerAddr, "address to run HTTP server")
	fs.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")

	args := make([]string, 0)
	serverPortSet := false
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-a" || os.Args[i] == "-b" {
			if i+1 < len(os.Args) {
				args = append(args, os.Args[i], os.Args[i+1])
				i++
			} else {
				args = append(args, os.Args[i])
			}
		}
		if port, exists := os.LookupEnv("SERVER_PORT"); exists && port != "" && !serverPortSet {
			log.Printf("Using server port from SERVER_PORT: %s", port)
			cfg.ServerAddr = fmt.Sprintf("localhost:%s", port)
			serverPortSet = true
		}
	}
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags, :%w", err)
	}
	if port, exists := os.LookupEnv("SERVER_PORT"); exists && port != "" {
		log.Printf("Using server port from SERVER_PORT: %s", port)
		cfg.ServerAddr = fmt.Sprintf("localhost:%s", port)
	} else if addr, exists := os.LookupEnv("SERVER_ADDRESS"); exists && addr != "" {
		log.Printf("Using server address from SERVER_ADDRESS: %s", addr)
		if !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
		cfg.ServerAddr = addr
	} else if fs.NFlag() == 0 {
		log.Printf("Using default server address: %s", cfg.ServerAddr)
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("https://%s/", cfg.ServerAddr)
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		log.Printf("Using default base URL: %s", cfg.BaseURL)
		cfg.BaseURL = "http://localhost:8080/"
	}
	if !strings.Contains(cfg.ServerAddr, ":") {
		log.Printf("Invalid ServerAddr, Using default server address: %s", cfg.ServerAddr)
		cfg.ServerAddr = "localhost:8080"
	}
	return cfg, nil
}
