package logcenter

import (
	"strings"
	"testing"
	"time"
)

func TestConfigFromEnvReadsSupportedVariables(t *testing.T) {
	t.Setenv("LOGCENTER_ENABLED", "true")
	t.Setenv("LOGCENTER_ENDPOINT", "<logcenter-endpoint>")
	t.Setenv("LOGCENTER_API_KEY", "test-api-key")
	t.Setenv("LOGCENTER_ENVIRONMENT", "production")
	t.Setenv("APP_ENV", "ignored")
	t.Setenv("LOGCENTER_SERVICE", "orders-api")
	t.Setenv("LOGCENTER_VERSION", "1.2.3")
	t.Setenv("LOGCENTER_OUTBOX_PATH", "/tmp/logcenter-outbox.jsonl")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_ENABLED", "true")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_CHAIN_ID", "orders-chain")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_SECRET", "chain-secret")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_STATE_PATH", "/tmp/logcenter-chain.json")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_METADATA_KEY", "integrity")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_EVENT_TYPES", "audit_event,log_event")
	t.Setenv("LOGCENTER_TAMPER_EVIDENCE_CLASSIFICATIONS", "audit,critical")
	t.Setenv("LOGCENTER_TIMEOUT", "3s")
	t.Setenv("LOGCENTER_SEND_TIMEOUT", "2s")
	t.Setenv("LOGCENTER_FLUSH_TIMEOUT", "4s")
	t.Setenv("LOGCENTER_CLOSE_TIMEOUT", "5s")
	t.Setenv("LOGCENTER_BUFFER_SIZE", "500")
	t.Setenv("LOGCENTER_BATCH_SIZE", "50")
	t.Setenv("LOGCENTER_FLUSH_INTERVAL", "250ms")
	t.Setenv("LOGCENTER_RETRY_ATTEMPTS", "2")
	t.Setenv("LOGCENTER_SENSITIVE_KEY_FRAGMENTS", "custom_secret, session_id")
	t.Setenv("LOGCENTER_MAX_STRING_BYTES", "2000")
	t.Setenv("LOGCENTER_MAX_METADATA_BYTES", "3000")
	t.Setenv("LOGCENTER_MAX_DATA_BYTES", "4000")
	t.Setenv("LOGCENTER_MAX_AUDIT_VALUE_BYTES", "5000")
	t.Setenv("LOGCENTER_MAX_EVENT_BYTES", "6000")
	t.Setenv("LOGCENTER_MAX_BATCH_BYTES", "7000")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if config.Enabled == nil || !*config.Enabled {
		t.Fatalf("Enabled = %v, want true", config.Enabled)
	}
	if config.Endpoint != "<logcenter-endpoint>" {
		t.Fatalf("Endpoint = %q", config.Endpoint)
	}
	if config.APIKey != "test-api-key" {
		t.Fatalf("APIKey = %q", config.APIKey)
	}
	if config.Environment != "production" {
		t.Fatalf("Environment = %q", config.Environment)
	}
	if config.Service != "orders-api" || config.Version != "1.2.3" {
		t.Fatalf("service/version = %q/%q", config.Service, config.Version)
	}
	if config.OutboxPath != "/tmp/logcenter-outbox.jsonl" {
		t.Fatalf("OutboxPath = %q", config.OutboxPath)
	}
	if !config.TamperEvidence.Enabled ||
		config.TamperEvidence.ChainID != "orders-chain" ||
		config.TamperEvidence.Secret != "chain-secret" ||
		config.TamperEvidence.StatePath != "/tmp/logcenter-chain.json" ||
		config.TamperEvidence.MetadataKey != "integrity" ||
		len(config.TamperEvidence.EventTypes) != 2 ||
		len(config.TamperEvidence.Classifications) != 2 {
		t.Fatalf("TamperEvidence = %#v", config.TamperEvidence)
	}
	if config.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %s", config.Timeout)
	}
	if config.SendTimeout != 2*time.Second || config.FlushTimeout != 4*time.Second || config.CloseTimeout != 5*time.Second {
		t.Fatalf("timeouts = %s/%s/%s", config.SendTimeout, config.FlushTimeout, config.CloseTimeout)
	}
	if config.BufferSize != 500 || config.BatchSize != 50 {
		t.Fatalf("buffer/batch = %d/%d", config.BufferSize, config.BatchSize)
	}
	if config.FlushInterval != 250*time.Millisecond {
		t.Fatalf("FlushInterval = %s", config.FlushInterval)
	}
	if config.RetryAttempts != 2 {
		t.Fatalf("RetryAttempts = %d", config.RetryAttempts)
	}
	if len(config.SensitiveKeyFragments) != 2 || config.SensitiveKeyFragments[0] != "custom_secret" || config.SensitiveKeyFragments[1] != "session_id" {
		t.Fatalf("SensitiveKeyFragments = %#v", config.SensitiveKeyFragments)
	}
	if config.MaxStringBytes != 2000 || config.MaxMetadataBytes != 3000 || config.MaxDataBytes != 4000 || config.MaxAuditValueBytes != 5000 || config.MaxEventBytes != 6000 || config.MaxBatchBytes != 7000 {
		t.Fatalf("limits = %d/%d/%d/%d/%d/%d", config.MaxStringBytes, config.MaxMetadataBytes, config.MaxDataBytes, config.MaxAuditValueBytes, config.MaxEventBytes, config.MaxBatchBytes)
	}
}

func TestConfigFromEnvFallsBackToAPPEnv(t *testing.T) {
	t.Setenv("APP_ENV", "staging")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if config.Environment != "staging" {
		t.Fatalf("Environment = %q, want staging", config.Environment)
	}
}

func TestConfigFromEnvCanDisableClient(t *testing.T) {
	t.Setenv("LOGCENTER_ENABLED", "disabled")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	client := NewClient(config)
	if client.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if err := client.Flush(nil); err != nil {
		t.Fatalf("Flush() error = %v, want nil", err)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError string
	}{
		{name: "enabled", key: "LOGCENTER_ENABLED", value: "maybe", wantError: "LOGCENTER_ENABLED"},
		{name: "tamper enabled", key: "LOGCENTER_TAMPER_EVIDENCE_ENABLED", value: "maybe", wantError: "LOGCENTER_TAMPER_EVIDENCE_ENABLED"},
		{name: "timeout", key: "LOGCENTER_TIMEOUT", value: "slow", wantError: "LOGCENTER_TIMEOUT"},
		{name: "send timeout", key: "LOGCENTER_SEND_TIMEOUT", value: "slow", wantError: "LOGCENTER_SEND_TIMEOUT"},
		{name: "flush timeout", key: "LOGCENTER_FLUSH_TIMEOUT", value: "slow", wantError: "LOGCENTER_FLUSH_TIMEOUT"},
		{name: "close timeout", key: "LOGCENTER_CLOSE_TIMEOUT", value: "slow", wantError: "LOGCENTER_CLOSE_TIMEOUT"},
		{name: "buffer", key: "LOGCENTER_BUFFER_SIZE", value: "many", wantError: "LOGCENTER_BUFFER_SIZE"},
		{name: "batch", key: "LOGCENTER_BATCH_SIZE", value: "many", wantError: "LOGCENTER_BATCH_SIZE"},
		{name: "flush", key: "LOGCENTER_FLUSH_INTERVAL", value: "soon", wantError: "LOGCENTER_FLUSH_INTERVAL"},
		{name: "retry", key: "LOGCENTER_RETRY_ATTEMPTS", value: "twice", wantError: "LOGCENTER_RETRY_ATTEMPTS"},
		{name: "max string", key: "LOGCENTER_MAX_STRING_BYTES", value: "large", wantError: "LOGCENTER_MAX_STRING_BYTES"},
		{name: "max metadata", key: "LOGCENTER_MAX_METADATA_BYTES", value: "large", wantError: "LOGCENTER_MAX_METADATA_BYTES"},
		{name: "max data", key: "LOGCENTER_MAX_DATA_BYTES", value: "large", wantError: "LOGCENTER_MAX_DATA_BYTES"},
		{name: "max audit", key: "LOGCENTER_MAX_AUDIT_VALUE_BYTES", value: "large", wantError: "LOGCENTER_MAX_AUDIT_VALUE_BYTES"},
		{name: "max event", key: "LOGCENTER_MAX_EVENT_BYTES", value: "large", wantError: "LOGCENTER_MAX_EVENT_BYTES"},
		{name: "max batch", key: "LOGCENTER_MAX_BATCH_BYTES", value: "large", wantError: "LOGCENTER_MAX_BATCH_BYTES"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := configFromLookup(func(key string) (string, bool) {
				if key == tt.key {
					return tt.value, true
				}
				return "", false
			})
			if err == nil {
				t.Fatalf("configFromLookup() error = nil, config = %#v", config)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %q, want variable %s", err.Error(), tt.wantError)
			}
		})
	}
}
