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
	ServerAddr  string
	BaseURL     string
	FileStorage string
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		ServerAddr:  ":8080",
		BaseURL:     "http://localhost:8080",
		FileStorage: "urls.json",
	}

	fs := flag.NewFlagSet("my-url-shortener", flag.ContinueOnError)
	fs.StringVar(&cfg.ServerAddr, "a", cfg.ServerAddr, "address to run HTTP server")
	fs.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened links")
	fs.StringVar(&cfg.FileStorage, "f", cfg.FileStorage, "file storage path")

	if port, exists := os.LookupEnv("SERVER_PORT"); exists && port != "" {
		log.Printf("Using server port from SERVER_PORT: %s", port)
		cfg.ServerAddr = fmt.Sprintf("localhost:%s", port)
	}

	os.Args = []string{"./shortener", "-f", "/tmp/my_urls.json"}

	args := make([]string, 0)
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(os.Args[i], "-a=") || strings.HasPrefix(os.Args[i], "-b=") || strings.HasPrefix(os.Args[i], "-f=") {
			parts := strings.SplitN(os.Args[i], "=", 2)
			args = append(args, parts[0], parts[1])
		} else if os.Args[i] == "-a" || os.Args[i] == "-b" || os.Args[i] == "-f" {
			args = append(args, arg)
			if i+1 < len(os.Args) {
				args = append(args, os.Args[i+1])
				i++
			}
		}
	}
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}
	if addr, exists := os.LookupEnv("SERVER_ADDRESS"); exists && addr != "" {
		log.Printf("Using server address: %s", addr)
		cfg.ServerAddr = addr
	} else if fs.NFlag() == 0 && cfg.ServerAddr == ":8080" {
		log.Printf("Using default server address: %s", cfg.ServerAddr)
	}
	if URL, exists := os.LookupEnv("BASE_URL"); exists && URL != "" {
		log.Printf("Using base URL: %s", URL)
		cfg.BaseURL = URL
	} else {
		log.Printf("Using default base URL: %s", cfg.BaseURL)
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("https://%s/", cfg.ServerAddr)
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		log.Printf("Using default base URL: %s", cfg.BaseURL)
		cfg.BaseURL = "http://localhost:8080/"
	}
	return cfg, nil
}
