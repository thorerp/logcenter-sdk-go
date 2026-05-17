package logcenter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultTamperEvidenceMetadataKey = "logcenter_integrity"

type TamperEvidenceConfig struct {
	Enabled         bool
	ChainID         string
	Secret          string
	StatePath       string
	MetadataKey     string
	EventTypes      []string
	Classifications []string
}

type tamperEvidence struct {
	config       TamperEvidenceConfig
	eventTypes   map[string]struct{}
	classes      map[string]struct{}
	metadataKey  string
	previousHash string
	sequence     uint64
	mu           sync.Mutex
}

type tamperEvidenceState struct {
	ChainID      string `json:"chain_id"`
	PreviousHash string `json:"previous_hash"`
	Sequence     uint64 `json:"sequence"`
}

func newTamperEvidence(config TamperEvidenceConfig) (*tamperEvidence, error) {
	if !config.Enabled {
		return nil, nil
	}
	metadataKey := strings.TrimSpace(config.MetadataKey)
	if metadataKey == "" {
		metadataKey = defaultTamperEvidenceMetadataKey
	}
	evidence := &tamperEvidence{
		config:      config,
		eventTypes:  stringSet(config.EventTypes),
		classes:     stringSet(config.Classifications),
		metadataKey: metadataKey,
	}
	if strings.TrimSpace(evidence.config.ChainID) == "" {
		evidence.config.ChainID = "default"
	}
	if err := evidence.loadState(); err != nil {
		return nil, err
	}
	return evidence, nil
}

func (evidence *tamperEvidence) Apply(event Event) (Event, error) {
	if evidence == nil || !evidence.matches(event) {
		return event, nil
	}

	evidence.mu.Lock()
	defer evidence.mu.Unlock()

	canonicalHash, err := hashEventForEvidence(event, evidence.metadataKey)
	if err != nil {
		return event, err
	}
	sequence := evidence.sequence + 1
	previousHash := evidence.previousHash
	integrityHash := evidence.hashEvidence(sequence, previousHash, canonicalHash)

	if event.Metadata == nil {
		event.Metadata = Fields{}
	}
	event.Metadata[evidence.metadataKey] = Fields{
		"version":        "v1",
		"algorithm":      evidence.algorithm(),
		"chain_id":       evidence.config.ChainID,
		"sequence":       sequence,
		"previous_hash":  previousHash,
		"canonical_hash": canonicalHash,
		"hash":           integrityHash,
	}

	evidence.sequence = sequence
	evidence.previousHash = integrityHash
	if err := evidence.saveState(); err != nil {
		return event, err
	}
	return event, nil
}

func (evidence *tamperEvidence) matches(event Event) bool {
	if len(evidence.eventTypes) > 0 {
		if _, ok := evidence.eventTypes[event.EventType]; !ok {
			return false
		}
	}
	if len(evidence.classes) > 0 {
		if _, ok := evidence.classes[event.Classification]; !ok {
			return false
		}
	}
	return true
}

func (evidence *tamperEvidence) hashEvidence(sequence uint64, previousHash, canonicalHash string) string {
	message := fmt.Sprintf("%s\n%d\n%s\n%s", evidence.config.ChainID, sequence, previousHash, canonicalHash)
	if evidence.config.Secret != "" {
		mac := hmac.New(sha256.New, []byte(evidence.config.Secret))
		_, _ = mac.Write([]byte(message))
		return hex.EncodeToString(mac.Sum(nil))
	}
	sum := sha256.Sum256([]byte(message))
	return hex.EncodeToString(sum[:])
}

func (evidence *tamperEvidence) algorithm() string {
	if evidence.config.Secret != "" {
		return "hmac-sha256"
	}
	return "sha256"
}

func (evidence *tamperEvidence) loadState() error {
	if strings.TrimSpace(evidence.config.StatePath) == "" {
		return nil
	}
	body, err := os.ReadFile(evidence.config.StatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read tamper evidence state: %w", err)
	}
	var state tamperEvidenceState
	if err := json.Unmarshal(body, &state); err != nil {
		return fmt.Errorf("decode tamper evidence state: %w", err)
	}
	if state.ChainID != "" && state.ChainID != evidence.config.ChainID {
		return fmt.Errorf("tamper evidence state chain_id %q does not match config %q", state.ChainID, evidence.config.ChainID)
	}
	evidence.previousHash = state.PreviousHash
	evidence.sequence = state.Sequence
	return nil
}

func (evidence *tamperEvidence) saveState() error {
	if strings.TrimSpace(evidence.config.StatePath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(evidence.config.StatePath), 0o755); err != nil {
		return fmt.Errorf("create tamper evidence state directory: %w", err)
	}
	state := tamperEvidenceState{
		ChainID:      evidence.config.ChainID,
		PreviousHash: evidence.previousHash,
		Sequence:     evidence.sequence,
	}
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode tamper evidence state: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(evidence.config.StatePath), ".logcenter-tamper-*")
	if err != nil {
		return fmt.Errorf("create tamper evidence temp state: %w", err)
	}
	tempPath := temp.Name()
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write tamper evidence state: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close tamper evidence state: %w", err)
	}
	if err := replaceFile(tempPath, evidence.config.StatePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func hashEventForEvidence(event Event, metadataKey string) (string, error) {
	event.Metadata = cloneFieldsWithoutKey(event.Metadata, metadataKey)
	encoded, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("encode tamper evidence event: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func cloneFieldsWithoutKey(fields Fields, key string) Fields {
	if fields == nil {
		return nil
	}
	cloned := make(Fields, len(fields))
	for field, value := range fields {
		if field == key {
			continue
		}
		cloned[field] = value
	}
	return cloned
}

func stringSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}
