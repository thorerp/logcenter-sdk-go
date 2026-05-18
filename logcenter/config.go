package logcenter

import (
	"net/http"
	"runtime"
	"strings"
	"time"
)

const Version = "0.1.0"

const (
	DefaultMaxStringBytes     = 0
	DefaultMaxMetadataBytes   = 1024 * 1024
	DefaultMaxDataBytes       = 5 * 1024 * 1024
	DefaultMaxAuditValueBytes = 1024 * 1024
	DefaultMaxEventBytes      = 5 * 1024 * 1024
	DefaultMaxBatchBytes      = 20 * 1024 * 1024
	MaxJSONValueBytes         = DefaultMaxEventBytes
)

type Config struct {
	Enabled        *bool
	Endpoint       string
	APIKey         string
	Environment    string
	Service        string
	Version        string
	Timeout        time.Duration
	SendTimeout    time.Duration
	FlushTimeout   time.Duration
	CloseTimeout   time.Duration
	BufferSize     int
	BatchSize      int
	FlushInterval  time.Duration
	RetryAttempts  int
	HTTPClient     *http.Client
	Source         map[string]any
	Hooks          Hooks
	OutboxPath     string
	TamperEvidence TamperEvidenceConfig

	SensitiveKeyFragments []string
	MaxStringBytes        int
	MaxMetadataBytes      int
	MaxDataBytes          int
	MaxAuditValueBytes    int
	MaxEventBytes         int
	MaxBatchBytes         int
}

func Bool(value bool) *bool {
	return &value
}

func (config Config) isEnabled() bool {
	return config.Enabled == nil || *config.Enabled
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
	if config.SendTimeout <= 0 {
		config.SendTimeout = config.Timeout
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
	if config.MaxStringBytes <= 0 {
		config.MaxStringBytes = DefaultMaxStringBytes
	}
	if config.MaxMetadataBytes <= 0 {
		config.MaxMetadataBytes = DefaultMaxMetadataBytes
	}
	if config.MaxDataBytes <= 0 {
		config.MaxDataBytes = DefaultMaxDataBytes
	}
	if config.MaxAuditValueBytes <= 0 {
		config.MaxAuditValueBytes = DefaultMaxAuditValueBytes
	}
	if config.MaxEventBytes <= 0 {
		config.MaxEventBytes = DefaultMaxEventBytes
	}
	if config.MaxBatchBytes <= 0 {
		config.MaxBatchBytes = DefaultMaxBatchBytes
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
