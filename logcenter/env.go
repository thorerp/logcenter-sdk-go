package logcenter

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func ConfigFromEnv() (Config, error) {
	return configFromLookup(os.LookupEnv)
}

func configFromLookup(lookup func(string) (string, bool)) (Config, error) {
	var config Config
	if value, ok := envString(lookup, "LOGCENTER_ENABLED"); ok {
		enabled, err := parseEnvBool("LOGCENTER_ENABLED", value)
		if err != nil {
			return Config{}, err
		}
		config.Enabled = Bool(enabled)
	}

	config.Endpoint, _ = envString(lookup, "LOGCENTER_ENDPOINT")
	config.APIKey, _ = envString(lookup, "LOGCENTER_API_KEY")
	config.Environment, _ = envString(lookup, "LOGCENTER_ENVIRONMENT", "APP_ENV")
	config.Service, _ = envString(lookup, "LOGCENTER_SERVICE")
	config.Version, _ = envString(lookup, "LOGCENTER_VERSION")
	config.OutboxPath, _ = envString(lookup, "LOGCENTER_OUTBOX_PATH")
	if value, ok := envString(lookup, "LOGCENTER_TAMPER_EVIDENCE_ENABLED"); ok {
		enabled, err := parseEnvBool("LOGCENTER_TAMPER_EVIDENCE_ENABLED", value)
		if err != nil {
			return Config{}, err
		}
		config.TamperEvidence.Enabled = enabled
	}
	config.TamperEvidence.ChainID, _ = envString(lookup, "LOGCENTER_TAMPER_EVIDENCE_CHAIN_ID")
	config.TamperEvidence.Secret, _ = envString(lookup, "LOGCENTER_TAMPER_EVIDENCE_SECRET")
	config.TamperEvidence.StatePath, _ = envString(lookup, "LOGCENTER_TAMPER_EVIDENCE_STATE_PATH")
	config.TamperEvidence.MetadataKey, _ = envString(lookup, "LOGCENTER_TAMPER_EVIDENCE_METADATA_KEY")
	config.TamperEvidence.EventTypes = envList(lookup, "LOGCENTER_TAMPER_EVIDENCE_EVENT_TYPES")
	config.TamperEvidence.Classifications = envList(lookup, "LOGCENTER_TAMPER_EVIDENCE_CLASSIFICATIONS")
	config.SensitiveKeyFragments = envList(lookup, "LOGCENTER_SENSITIVE_KEY_FRAGMENTS")

	var err error
	if config.Timeout, err = envDuration(lookup, "LOGCENTER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if config.SendTimeout, err = envDuration(lookup, "LOGCENTER_SEND_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if config.FlushTimeout, err = envDuration(lookup, "LOGCENTER_FLUSH_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if config.CloseTimeout, err = envDuration(lookup, "LOGCENTER_CLOSE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if config.BufferSize, err = envInt(lookup, "LOGCENTER_BUFFER_SIZE"); err != nil {
		return Config{}, err
	}
	if config.BatchSize, err = envInt(lookup, "LOGCENTER_BATCH_SIZE"); err != nil {
		return Config{}, err
	}
	if config.FlushInterval, err = envDuration(lookup, "LOGCENTER_FLUSH_INTERVAL"); err != nil {
		return Config{}, err
	}
	if config.RetryAttempts, err = envInt(lookup, "LOGCENTER_RETRY_ATTEMPTS"); err != nil {
		return Config{}, err
	}
	if config.MaxStringBytes, err = envInt(lookup, "LOGCENTER_MAX_STRING_BYTES"); err != nil {
		return Config{}, err
	}
	if config.MaxMetadataBytes, err = envInt(lookup, "LOGCENTER_MAX_METADATA_BYTES"); err != nil {
		return Config{}, err
	}
	if config.MaxDataBytes, err = envInt(lookup, "LOGCENTER_MAX_DATA_BYTES"); err != nil {
		return Config{}, err
	}
	if config.MaxAuditValueBytes, err = envInt(lookup, "LOGCENTER_MAX_AUDIT_VALUE_BYTES"); err != nil {
		return Config{}, err
	}
	if config.MaxEventBytes, err = envInt(lookup, "LOGCENTER_MAX_EVENT_BYTES"); err != nil {
		return Config{}, err
	}
	return config, nil
}

func envList(lookup func(string) (string, bool), name string) []string {
	value, ok := envString(lookup, name)
	if !ok {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func envString(lookup func(string) (string, bool), names ...string) (string, bool) {
	for _, name := range names {
		value, ok := lookup(name)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func envInt(lookup func(string) (string, bool), name string) (int, error) {
	value, ok := envString(lookup, name)
	if !ok {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func envDuration(lookup func(string) (string, bool), name string) (time.Duration, error) {
	value, ok := envString(lookup, name)
	if !ok {
		return 0, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration, for example 2s or 500ms: %w", name, err)
	}
	return parsed, nil
}

func parseEnvBool(name, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on", "enabled":
		return true, nil
	case "0", "f", "false", "n", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", name)
	}
}
