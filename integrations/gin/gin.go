package logcentergin

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

type Option func(*config)

type config struct {
	routeTemplateFunc func(*gin.Context) string
	operationFunc     func(*gin.Context) string
	userIDFunc        func(*gin.Context) string
	tenantIDFunc      func(*gin.Context) string
	metadataFunc      func(*gin.Context) logcenter.Fields
	dataFunc          func(*gin.Context) logcenter.Fields
	bodyCaptureFunc   func(*gin.Context) bool
	bodyCapture       logcenter.RequestBodyCaptureOptions
	panicCode         string
}

func RouteTemplateFunc(fn func(*gin.Context) string) Option {
	return func(config *config) {
		config.routeTemplateFunc = fn
	}
}

func OperationFunc(fn func(*gin.Context) string) Option {
	return func(config *config) {
		config.operationFunc = fn
	}
}

func UserIDFunc(fn func(*gin.Context) string) Option {
	return func(config *config) {
		config.userIDFunc = fn
	}
}

func TenantIDFunc(fn func(*gin.Context) string) Option {
	return func(config *config) {
		config.tenantIDFunc = fn
	}
}

func MetadataFunc(fn func(*gin.Context) logcenter.Fields) Option {
	return func(config *config) {
		config.metadataFunc = fn
	}
}

func DataFunc(fn func(*gin.Context) logcenter.Fields) Option {
	return func(config *config) {
		config.dataFunc = fn
	}
}

func RequestBodyCapture(maxBytes int64, contentTypes ...string) Option {
	return RequestBodyCaptureFunc(func(*gin.Context) bool {
		return true
	}, maxBytes, contentTypes...)
}

func RequestBodyCaptureFunc(fn func(*gin.Context) bool, maxBytes int64, contentTypes ...string) Option {
	return func(config *config) {
		config.bodyCaptureFunc = fn
		config.bodyCapture = logcenter.RequestBodyCaptureOptions{
			MaxBytes:     maxBytes,
			ContentTypes: append([]string(nil), contentTypes...),
		}
	}
}

func PanicCode(code string) Option {
	return func(config *config) {
		config.panicCode = code
	}
}

func Middleware(client *logcenter.Client, options ...Option) gin.HandlerFunc {
	config := newConfig(options...)
	return func(c *gin.Context) {
		requestID := firstHeader(c.Request, "X-LogCenter-Request-Id", "X-Request-Id")
		traceID, parentSpanID := traceFromRequest(c.Request)
		routeTemplate := config.routeTemplate(c)
		operation := config.operation(c, routeTemplate)

		ctx, request := client.StartRequest(c.Request.Context(), logcenter.RequestStartOptions{
			RequestID:     requestID,
			TraceID:       traceID,
			SpanID:        parentSpanID,
			UserID:        config.userID(c),
			TenantID:      config.tenantID(c),
			Operation:     operation,
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			RouteTemplate: routeTemplate,
			Metadata: mergeFields(logcenter.Fields{
				"client_ip":  c.ClientIP(),
				"user_agent": c.Request.UserAgent(),
			}, config.metadata(c)),
			Data: config.data(c),
		})
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		statusCode := c.Writer.Status()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		status := logcenter.StatusSuccess
		if statusCode >= http.StatusInternalServerError {
			status = logcenter.StatusFailed
		}
		request.EndWithContext(c.Request.Context(), logcenter.RequestEndOptions{
			Status:     status,
			HTTPStatus: &statusCode,
		})
	}
}

func Recovery(client *logcenter.Client, options ...Option) gin.HandlerFunc {
	config := newConfig(options...)
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			routeTemplate := config.routeTemplate(c)
			err := fmt.Errorf("panic: %v", recovered)
			code := config.panicCode
			if code == "" {
				code = "PANIC"
			}
			client.RecordError(c.Request.Context(), err, logcenter.ErrorOptions{
				Code:       code,
				Type:       "panic",
				Severity:   logcenter.SeverityError,
				StackTrace: string(debug.Stack()),
				Metadata: mergeFields(logcenter.Fields{
					"method":         c.Request.Method,
					"path":           c.Request.URL.Path,
					"route_template": routeTemplate,
					"client_ip":      c.ClientIP(),
				}, config.metadata(c)),
				Data: config.data(c),
			})

			if !c.Writer.Written() {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			c.Abort()
		}()
		c.Next()
	}
}

func SetRequestContext(c *gin.Context, request logcenter.RequestContext) {
	c.Request = c.Request.WithContext(logcenter.ContextWithRequest(c.Request.Context(), request))
}

func SetUserID(c *gin.Context, userID string) {
	c.Request = c.Request.WithContext(logcenter.ContextWithUser(c.Request.Context(), userID))
}

func SetTenantID(c *gin.Context, tenantID string) {
	c.Request = c.Request.WithContext(logcenter.ContextWithTenant(c.Request.Context(), tenantID))
}

func SetIdentity(c *gin.Context, userID, tenantID string) {
	SetUserID(c, userID)
	SetTenantID(c, tenantID)
}

func SetOperation(c *gin.Context, operation string) {
	c.Request = c.Request.WithContext(logcenter.ContextWithOperation(c.Request.Context(), operation))
}

func newConfig(options ...Option) config {
	config := config{}
	for _, option := range options {
		option(&config)
	}
	return config
}

func (config config) routeTemplate(c *gin.Context) string {
	if config.routeTemplateFunc != nil {
		if routeTemplate := strings.TrimSpace(config.routeTemplateFunc(c)); routeTemplate != "" {
			return routeTemplate
		}
	}
	if fullPath := c.FullPath(); fullPath != "" {
		return fullPath
	}
	return c.Request.URL.Path
}

func (config config) operation(c *gin.Context, routeTemplate string) string {
	if config.operationFunc != nil {
		if operation := strings.TrimSpace(config.operationFunc(c)); operation != "" {
			return operation
		}
	}
	return c.Request.Method + " " + routeTemplate
}

func (config config) userID(c *gin.Context) string {
	if config.userIDFunc == nil {
		return ""
	}
	return config.userIDFunc(c)
}

func (config config) tenantID(c *gin.Context) string {
	if config.tenantIDFunc == nil {
		return ""
	}
	return config.tenantIDFunc(c)
}

func (config config) metadata(c *gin.Context) logcenter.Fields {
	if config.metadataFunc == nil {
		return nil
	}
	return config.metadataFunc(c)
}

func (config config) data(c *gin.Context) logcenter.Fields {
	data := logcenter.Fields(nil)
	if config.dataFunc == nil {
		data = nil
	} else {
		data = config.dataFunc(c)
	}
	if config.bodyCaptureFunc == nil || !config.bodyCaptureFunc(c) {
		return data
	}
	body, ok := logcenter.CaptureHTTPRequestBody(c.Request, config.bodyCapture)
	if !ok {
		return data
	}
	return mergeFields(data, body)
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		value := strings.TrimSpace(r.Header.Get(name))
		if value != "" {
			return value
		}
	}
	return ""
}

func traceFromRequest(r *http.Request) (string, string) {
	traceID := firstHeader(r, "X-LogCenter-Trace-Id", "X-Trace-Id")
	if traceID != "" {
		return traceID, ""
	}
	traceparent := firstHeader(r, "Traceparent")
	parts := strings.Split(traceparent, "-")
	if len(parts) == 4 && strings.EqualFold(parts[0], "00") {
		return parts[1], parts[2]
	}
	return "", ""
}

func mergeFields(left, right logcenter.Fields) logcenter.Fields {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	merged := make(logcenter.Fields, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		merged[key] = value
	}
	return merged
}
