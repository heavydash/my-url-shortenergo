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
		assert.Equal(t, ":9090", cfg.ServerAddr)
		assert.Equal(t, "http://example.com", cfg.BaseURL)
	})
	//Переменные не заданы, флаги заданы
	t.Run("flags override env variables", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test", "-a", ":8081", "-b", "http://test.com"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, ":8081", cfg.ServerAddr)
		assert.Equal(t, "http://test.com", cfg.BaseURL)
	})
	//Ничего не задано
	t.Run("default values", func(t *testing.T) {
		originalArgs := os.Args
		os.Args = []string{"test"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, ":8080", cfg.ServerAddr)
		assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	})
	//Пустые переменные окружения
	t.Run("env are empty", func(t *testing.T) {
		os.Setenv("SERVER_ADDRESS", "")
		os.Setenv("BASE_URL", "")
		defer os.Unsetenv("SERVER_ADDRESS")
		defer os.Unsetenv("BASE_URL")

		originalArgs := os.Args
		os.Args = []string{"test", "-a", ":8081", "-b", "http://test.com"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, ":8081", cfg.ServerAddr)
		assert.Equal(t, "http://test.com", cfg.BaseURL)
	})
	//Валидация
	t.Run("Base URL is empty", func(t *testing.T) {
		os.Setenv("SERVER_ADDRESS", ":9090")
		os.Setenv("BASE_URL", "")
		defer os.Unsetenv("SERVER_ADDRESS")
		defer os.Unsetenv("BASE_URL")

		originalArgs := os.Args
		os.Args = []string{"test"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, ":9090", cfg.ServerAddr)
		assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	})
	//SERVER_PORT задан
	t.Run("server_port_set", func(t *testing.T) {
		os.Setenv("SERVER_PORT", "33507")
		os.Setenv("BASE_URL", "http://example.com")
		defer os.Unsetenv("SERVER_PORT")
		defer os.Unsetenv("BASE_URL")

		originalArgs := os.Args
		os.Args = []string{"test"}
		defer func() { os.Args = originalArgs }()

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, "localhost:33507", cfg.ServerAddr)
		assert.Equal(t, "http://example.com", cfg.BaseURL)
	})

}
