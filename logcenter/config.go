package logcenter

import (
	"net/http"
	"runtime"
	"strings"
	"time"
)

const Version = "0.1.0"

const (
	DefaultMaxStringBytes     = 8 * 1024
	DefaultMaxMetadataBytes   = 64 * 1024
	DefaultMaxDataBytes       = 64 * 1024
	DefaultMaxAuditValueBytes = 64 * 1024
	DefaultMaxEventBytes      = 256 * 1024
	MaxJSONValueBytes         = 64 * 1024
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
	if config.MaxMetadataBytes <= 0 || config.MaxMetadataBytes > MaxJSONValueBytes {
		config.MaxMetadataBytes = DefaultMaxMetadataBytes
	}
	if config.MaxDataBytes <= 0 || config.MaxDataBytes > MaxJSONValueBytes {
		config.MaxDataBytes = DefaultMaxDataBytes
	}
	if config.MaxAuditValueBytes <= 0 || config.MaxAuditValueBytes > MaxJSONValueBytes {
		config.MaxAuditValueBytes = DefaultMaxAuditValueBytes
	}
	if config.MaxEventBytes <= 0 {
		config.MaxEventBytes = DefaultMaxEventBytes
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
