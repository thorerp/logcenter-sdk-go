package logcentergin_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	logcentergin "github.com/thorerp/logcenter-sdk-go/integrations/gin"
	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

type capturedBatch struct {
	Events []logcenter.Event `json:"events"`
}

func TestMiddlewareCollectsRouteStatusAndEnrichedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	received := make(chan capturedBatch, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":3,"accepted":3,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer collector.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      collector.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "gin-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	router := gin.New()
	router.Use(logcentergin.Middleware(client))
	router.Use(func(c *gin.Context) {
		logcentergin.SetIdentity(c, "user-123", "tenant-123")
		c.Next()
	})
	router.GET("/orders/:id", func(c *gin.Context) {
		client.Info(c.Request.Context(), "inside gin handler", nil)
		c.Status(http.StatusAccepted)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/123", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	if len(batch.Events) != 3 {
		t.Fatalf("events = %d, want request start, log, request finish", len(batch.Events))
	}
	started := batch.Events[0]
	if started.EventType != logcenter.EventTypeRequestStarted || started.RouteTemplate != "/orders/:id" {
		t.Fatalf("started = %#v, want gin route template", started)
	}
	if started.Metadata["client_ip"] == "" {
		t.Fatalf("started metadata = %#v, want client_ip", started.Metadata)
	}
	logEvent := batch.Events[1]
	if logEvent.UserID != "user-123" || logEvent.TenantID != "tenant-123" {
		t.Fatalf("log identity = %#v, want enriched identity", logEvent)
	}
	finished := batch.Events[2]
	if finished.EventType != logcenter.EventTypeRequestFinished || finished.RouteTemplate != "/orders/:id" {
		t.Fatalf("finished = %#v, want gin route template", finished)
	}
	if finished.HTTPStatus == nil || *finished.HTTPStatus != http.StatusAccepted {
		t.Fatalf("finished http status = %v, want 202", finished.HTTPStatus)
	}
	if finished.UserID != "user-123" || finished.TenantID != "tenant-123" {
		t.Fatalf("finished identity = %#v, want enriched identity", finished)
	}
}

func TestRecoveryCapturesPanicWithStackTraceAndStatus500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	received := make(chan capturedBatch, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":3,"accepted":3,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer collector.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      collector.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "gin-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	router := gin.New()
	router.Use(logcentergin.Middleware(client))
	router.Use(func(c *gin.Context) {
		logcentergin.SetIdentity(c, "user-123", "tenant-123")
		c.Next()
	})
	router.Use(logcentergin.Recovery(client))
	router.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	batch := <-received
	var errorEvent *logcenter.Event
	var finished *logcenter.Event
	for i := range batch.Events {
		event := &batch.Events[i]
		if event.EventType == logcenter.EventTypeErrorEvent {
			errorEvent = event
		}
		if event.EventType == logcenter.EventTypeRequestFinished {
			finished = event
		}
	}
	if errorEvent == nil {
		t.Fatalf("events = %#v, want panic error event", batch.Events)
	}
	if errorEvent.ErrorCode != "PANIC" || errorEvent.ErrorType != "panic" || errorEvent.StackTrace == "" {
		t.Fatalf("error event = %#v, want panic code/type/stack", errorEvent)
	}
	if errorEvent.UserID != "user-123" || errorEvent.TenantID != "tenant-123" {
		t.Fatalf("error identity = %#v, want enriched identity", errorEvent)
	}
	if finished == nil || finished.HTTPStatus == nil || *finished.HTTPStatus != http.StatusInternalServerError || finished.Status != logcenter.StatusFailed {
		t.Fatalf("finished = %#v, want failed 500", finished)
	}
}

func TestMiddlewareCapturesAllowedRequestBodyAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	received := make(chan capturedBatch, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received <- batch
		_, _ = w.Write([]byte(`{"batch_id":"ok","received":2,"accepted":2,"duplicated":0,"rejected":0,"errors":[]}`))
	}))
	defer collector.Close()

	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      collector.URL,
		APIKey:        "test-api-key",
		Environment:   "test",
		Service:       "gin-api",
		FlushInterval: time.Hour,
		BufferSize:    10,
		BatchSize:     10,
	})
	defer client.Close(context.Background())

	router := gin.New()
	router.Use(logcentergin.Middleware(client,
		logcentergin.RequestBodyCaptureFunc(func(c *gin.Context) bool {
			return c.FullPath() == "/orders"
		}, 1024, "application/json"),
	))
	router.POST("/orders", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if !strings.Contains(string(body), "secret") {
			t.Fatalf("handler body = %q, want original body restored", body)
		}
		client.Info(c.Request.Context(), "body consumed", nil)
		c.Status(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"name":"visible","api_key":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	started := (<-received).Events[0]
	body := started.Data["request_body"].(map[string]any)
	value := body["value"].(map[string]any)
	if value["api_key"] != "[REDACTED]" || value["name"] != "visible" {
		t.Fatalf("captured body = %#v, want redacted key and visible safe field", value)
	}
}
