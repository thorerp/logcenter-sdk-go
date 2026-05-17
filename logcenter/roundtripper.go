package logcenter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RoundTripperOption func(*roundTripperConfig)

type RoundTripMetadataFunc func(*http.Request, *http.Response, error) Fields
type RoundTripDataFunc func(*http.Request, *http.Response, error) Fields

type roundTripperConfig struct {
	spanNameFunc func(*http.Request) string
	metadataFunc RoundTripMetadataFunc
	dataFunc     RoundTripDataFunc
	errorCode    string
}

type InstrumentedRoundTripper struct {
	client *Client
	base   http.RoundTripper
	config roundTripperConfig
}

func RoundTripperSpanName(name string) RoundTripperOption {
	return RoundTripperSpanNameFunc(func(*http.Request) string {
		return name
	})
}

func RoundTripperSpanNameFunc(fn func(*http.Request) string) RoundTripperOption {
	return func(config *roundTripperConfig) {
		config.spanNameFunc = fn
	}
}

func RoundTripperMetadataFunc(fn RoundTripMetadataFunc) RoundTripperOption {
	return func(config *roundTripperConfig) {
		config.metadataFunc = fn
	}
}

func RoundTripperDataFunc(fn RoundTripDataFunc) RoundTripperOption {
	return func(config *roundTripperConfig) {
		config.dataFunc = fn
	}
}

func RoundTripperErrorCode(code string) RoundTripperOption {
	return func(config *roundTripperConfig) {
		config.errorCode = code
	}
}

func (client *Client) RoundTripper(base http.RoundTripper, options ...RoundTripperOption) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	config := roundTripperConfig{}
	for _, option := range options {
		option(&config)
	}
	return &InstrumentedRoundTripper{
		client: client,
		base:   base,
		config: config,
	}
}

func (transport *InstrumentedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("logcenter roundtripper: nil request")
	}

	ctx, requestContext := ensureRequestContext(req.Context())
	parentSpanID := requestContext.SpanID
	spanID := newID("spn_")
	requestContext.SpanID = spanID
	req = req.WithContext(ContextWithRequest(ctx, requestContext))

	startedAt := time.Now().UTC()
	resp, err := transport.base.RoundTrip(req)
	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	metadata := transport.metadata(req, resp, err)
	data := transport.data(req, resp, err)
	status := roundTripStatus(req.Context(), resp, err)
	errorCode, errorMessage := "", ""
	if err != nil {
		errorCode = transport.errorCode()
		errorMessage = err.Error()
		transport.client.RecordError(ContextWithRequest(context.Background(), requestContext), err, ErrorOptions{
			Code:     errorCode,
			Type:     "http_client",
			Severity: SeverityError,
			Metadata: metadata,
			Data:     data,
		})
	}

	var httpStatus *int
	if resp != nil {
		statusCode := resp.StatusCode
		httpStatus = &statusCode
	}

	transport.client.enqueue(Event{
		EventType:    EventTypeSpan,
		RequestID:    requestContext.RequestID,
		TraceID:      requestContext.TraceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		UserID:       requestContext.UserID,
		TenantID:     requestContext.TenantID,
		Operation:    requestContext.Operation,
		Name:         transport.spanName(req),
		Kind:         "client",
		Status:       status,
		HTTPStatus:   httpStatus,
		StartedAt:    formatTime(startedAt),
		FinishedAt:   formatTime(finishedAt),
		DurationMS:   &duration,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Metadata:     metadata,
		Data:         data,
	})

	return resp, err
}

func (transport *InstrumentedRoundTripper) spanName(req *http.Request) string {
	if transport.config.spanNameFunc != nil {
		if name := strings.TrimSpace(transport.config.spanNameFunc(req)); name != "" {
			return name
		}
	}
	if req.Method == "" {
		return "HTTP request"
	}
	return "HTTP " + req.Method
}

func (transport *InstrumentedRoundTripper) metadata(req *http.Request, resp *http.Response, err error) Fields {
	fields := Fields{
		"method": req.Method,
		"scheme": req.URL.Scheme,
		"host":   req.URL.Host,
		"path":   req.URL.Path,
	}
	if resp != nil {
		fields["status_code"] = resp.StatusCode
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	return mergeFields(fields, transport.extraMetadata(req, resp, err))
}

func (transport *InstrumentedRoundTripper) extraMetadata(req *http.Request, resp *http.Response, err error) Fields {
	if transport.config.metadataFunc == nil {
		return nil
	}
	return transport.config.metadataFunc(req, resp, err)
}

func (transport *InstrumentedRoundTripper) data(req *http.Request, resp *http.Response, err error) Fields {
	if transport.config.dataFunc == nil {
		return nil
	}
	return transport.config.dataFunc(req, resp, err)
}

func (transport *InstrumentedRoundTripper) errorCode() string {
	if strings.TrimSpace(transport.config.errorCode) != "" {
		return transport.config.errorCode
	}
	return "HTTP_CLIENT_ERROR"
}

func roundTripStatus(ctx context.Context, resp *http.Response, err error) string {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return StatusCanceled
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return StatusTimeout
		}
		return StatusFailed
	}
	if resp != nil && resp.StatusCode >= http.StatusInternalServerError {
		return StatusFailed
	}
	return StatusSuccess
}

func (transport *InstrumentedRoundTripper) String() string {
	return fmt.Sprintf("logcenter.InstrumentedRoundTripper{%T}", transport.base)
}
