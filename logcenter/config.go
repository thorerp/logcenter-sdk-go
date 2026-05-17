package logcenter

import (
	"net/http"
	"runtime"
	"strings"
	"time"
)

const Version = "0.1.0"

type Config struct {
	Endpoint      string
	APIKey        string
	Environment   string
	Service       string
	Version       string
	Timeout       time.Duration
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
	RetryAttempts int
	HTTPClient    *http.Client
	Source        map[string]any
}

func (config Config) normalized() Config {
	config.Endpoint = strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if strings.TrimSpace(config.Environment) == "" {
		config.Environment = "development"
	}
	if strings.TrimSpace(config.Service) == "" {
		config.Service = "go-service"
	}
	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Second
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.BatchSize > config.BufferSize {
		config.BatchSize = config.BufferSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = time.Second
	}
	if config.RetryAttempts < 0 {
		config.RetryAttempts = 0
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.Source == nil {
		config.Source = defaultSource()
	}
	return config
}

func defaultSource() map[string]any {
	return map[string]any{
		"sdk":         "logcenter-go",
		"sdk_version": Version,
		"runtime":     runtime.Version(),
	}
}
