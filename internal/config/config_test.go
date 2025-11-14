package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	///Переменные окружения заданы
	t.Run("env variables set", func(t *testing.T) {
		_ = os.Setenv("SERVER_ADDRESS", ":9090")
		os.Setenv("BASE_URL", "http://example.com")
		defer os.Unsetenv("SERVER_ADDRESS")
		defer os.Unsetenv("BASE_URL")

		originalArgs := os.Args
		os.Args = []string{"test"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Contains(t, []string{":9090", "localhost:9090"}, cfg.ServerAddr)
		assert.Equal(t, "http://example.com", cfg.BaseURL)
	})
	//Ничего не задано
	t.Run("default values", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Contains(t, []string{":8081", "localhost:8081"}, cfg.ServerAddr)
		assert.Equal(t, "http://localhost:8081", cfg.BaseURL)
	})
}
func TestNewConfig_FileStoragePath(t *testing.T) {

	t.Run("from flag -f", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test", "-f", "/tmp/urls.json"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/urls.json", cfg.FileStoragePath)
	})
	t.Run("empty flag", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test", "-f", ""}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, "", cfg.FileStoragePath)
	})
	t.Run("empty flag", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test", "-f", ""}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, "", cfg.FileStoragePath)
	})
}
