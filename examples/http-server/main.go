package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func main() {
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      os.Getenv("LOGCENTER_ENDPOINT"),
		APIKey:        os.Getenv("LOGCENTER_API_KEY"),
		Environment:   getenv("APP_ENV", "development"),
		Service:       "example-http-server",
		Version:       "0.1.0",
		Timeout:       2 * time.Second,
		BufferSize:    1000,
		BatchSize:     100,
		FlushInterval: time.Second,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := client.StartSpan(r.Context(), "charge-card", logcenter.SpanKind("client"))
		defer span.End(logcenter.StatusSuccess)

		client.Info(ctx, "checkout started", logcenter.Fields{"route": "/checkout"})
		client.Audit(ctx, logcenter.AuditEvent{
			ActorType:  "system",
			ActorID:    "example",
			Action:     "checkout.approved",
			EntityType: "checkout",
			EntityID:   "demo",
			Changes: []logcenter.Change{
				{Field: "status", OldValue: "pending", NewValue: "approved"},
			},
		})

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		err := errors.New("provider timeout")
		client.RecordError(r.Context(), err, logcenter.ErrorOptions{Code: "PROVIDER_TIMEOUT", Type: "provider_timeout"})
		http.Error(w, err.Error(), http.StatusBadGateway)
	})

	server := &http.Server{
		Addr:    ":8090",
		Handler: client.HTTPMiddleware()(mux),
	}

	go func() {
		log.Printf("example server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = client.Close(ctx)
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
