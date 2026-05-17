package logcenter

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type durableOutbox struct {
	path string
	mu   sync.Mutex
}

func newDurableOutbox(path string) *durableOutbox {
	if path == "" {
		return nil
	}
	return &durableOutbox{path: path}
}

func (outbox *durableOutbox) Append(events []Event) error {
	if outbox == nil || len(events) == 0 {
		return nil
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(outbox.path), 0o755); err != nil {
		return fmt.Errorf("create outbox directory: %w", err)
	}
	file, err := os.OpenFile(outbox.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open outbox: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return fmt.Errorf("append outbox event: %w", err)
		}
	}
	return nil
}

func (outbox *durableOutbox) Peek(max int) ([]Event, error) {
	if outbox == nil {
		return nil, nil
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()

	file, err := os.Open(outbox.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open outbox: %w", err)
	}
	defer file.Close()

	events := make([]Event, 0, max)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), DefaultMaxEventBytes*2)
	for scanner.Scan() {
		if max > 0 && len(events) >= max {
			break
		}
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan outbox: %w", err)
	}
	return events, nil
}

func (outbox *durableOutbox) Remove(eventIDs []string) error {
	if outbox == nil || len(eventIDs) == 0 {
		return nil
	}
	outbox.mu.Lock()
	defer outbox.mu.Unlock()

	ids := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID != "" {
			ids[eventID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	source, err := os.Open(outbox.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open outbox: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outbox.path), 0o755); err != nil {
		return fmt.Errorf("create outbox directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(outbox.path), ".logcenter-outbox-*")
	if err != nil {
		return fmt.Errorf("create temp outbox: %w", err)
	}
	tempPath := temp.Name()
	keepCount := 0
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64*1024), DefaultMaxEventBytes*2)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event Event
		if err := json.Unmarshal(line, &event); err == nil {
			if _, remove := ids[event.EventID]; remove {
				continue
			}
		}
		if _, err := temp.Write(append(line, '\n')); err != nil {
			_ = temp.Close()
			_ = os.Remove(tempPath)
			return fmt.Errorf("write temp outbox: %w", err)
		}
		keepCount++
	}
	if err := scanner.Err(); err != nil {
		_ = source.Close()
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("scan outbox: %w", err)
	}
	if err := source.Close(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("close outbox: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp outbox: %w", err)
	}
	if keepCount == 0 {
		_ = os.Remove(tempPath)
		if err := os.Remove(outbox.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty outbox: %w", err)
		}
		return nil
	}
	if err := replaceFile(tempPath, outbox.path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func replaceFile(source, target string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old outbox: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("replace outbox: %w", err)
	}
	return nil
}

func eventIDs(events []Event) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if event.EventID != "" {
			ids = append(ids, event.EventID)
		}
	}
	return ids
}

func copyEvents(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	copied := make([]Event, len(events))
	copy(copied, events)
	return copied
}
