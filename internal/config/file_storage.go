package config

import (
	"os"
	"strings"
)

type FileStorageConfig struct {
	Path string
}

func NewFileStorageConfig() *FileStorageConfig {
	cfg := &FileStorageConfig{}

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "-f=") {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				cfg.Path = parts[1]
			}
		} else if arg == "-f" && i+1 < len(os.Args) {
			cfg.Path = os.Args[i+1]
			i++
		}
	}

	if env := os.Getenv("FILE_STORAGE_PATH"); env != "" {
		cfg.Path = env
	}

	return cfg
}
