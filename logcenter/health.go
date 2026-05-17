package logcenter

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

const (
	HealthStatusOK       = "ok"
	HealthStatusDegraded = "degraded"
	HealthStatusDisabled = "disabled"
)

type HealthReport struct {
	Status      string `json:"status"`
	Enabled     bool   `json:"enabled"`
	SDKVersion  string `json:"sdk_version"`
	Runtime     string `json:"runtime"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	QueueLength int    `json:"queue_length"`
	CheckedAt   string `json:"checked_at"`
	Stats       Stats  `json:"stats"`
}

type HealthHandlerOptions struct {
	DegradedStatusCode int
	DisabledStatusCode int
}

func (client *Client) Health() HealthReport {
	stats := client.Stats()
	status := healthStatus(client.Enabled(), stats)
	queueLength := 0
	if client.queue != nil {
		queueLength = client.queue.len()
	}

	return HealthReport{
		Status:      status,
		Enabled:     client.Enabled(),
		SDKVersion:  Version,
		Runtime:     runtime.Version(),
		Service:     client.config.Service,
		Environment: client.config.Environment,
		QueueLength: queueLength,
		CheckedAt:   formatTime(time.Now().UTC()),
		Stats:       stats,
	}
}

func (client *Client) HealthHandler(options ...HealthHandlerOptions) http.Handler {
	handlerOptions := HealthHandlerOptions{}
	if len(options) > 0 {
		handlerOptions = options[0]
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		report := client.Health()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(healthHTTPStatus(report.Status, handlerOptions))
		_ = json.NewEncoder(w).Encode(report)
	})
}

func healthStatus(enabled bool, stats Stats) string {
	if !enabled {
		return HealthStatusDisabled
	}
	if stats.Dropped > 0 ||
		stats.FailedEvents > 0 ||
		stats.FailedBatches > 0 ||
		stats.Rejected > 0 ||
		stats.LastError != "" {
		return HealthStatusDegraded
	}
	return HealthStatusOK
}

func healthHTTPStatus(status string, options HealthHandlerOptions) int {
	switch status {
	case HealthStatusDegraded:
		if options.DegradedStatusCode > 0 {
			return options.DegradedStatusCode
		}
	case HealthStatusDisabled:
		if options.DisabledStatusCode > 0 {
			return options.DisabledStatusCode
		}
	}
	return http.StatusOK
}
