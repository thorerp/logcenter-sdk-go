package logcenter

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

const DefaultRequestBodyCaptureMaxBytes int64 = 8 * 1024

type RequestBodyCaptureOptions struct {
	MaxBytes     int64
	ContentTypes []string
}

func CaptureHTTPRequestBody(r *http.Request, options RequestBodyCaptureOptions) (Fields, bool) {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return nil, false
	}
	contentType := r.Header.Get("Content-Type")
	mediaType := mediaTypeOf(contentType)
	if !contentTypeAllowed(mediaType, options.ContentTypes) {
		return nil, false
	}

	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultRequestBodyCaptureMaxBytes
	}

	captured, truncated, ok := readAndRestoreBody(r, maxBytes)
	if !ok || len(captured) == 0 {
		return nil, false
	}

	value, encoding := decodeCapturedBody(captured, mediaType, truncated)
	return Fields{
		"request_body": Fields{
			"content_type": contentType,
			"encoding":     encoding,
			"size_bytes":   len(captured),
			"max_bytes":    maxBytes,
			"truncated":    truncated,
			"value":        value,
		},
	}, true
}

func readAndRestoreBody(r *http.Request, maxBytes int64) ([]byte, bool, bool) {
	original := r.Body
	limit := maxBytes + 1
	buffer, err := io.ReadAll(io.LimitReader(original, limit))
	r.Body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(buffer), original),
		closer: original,
	}
	if err != nil {
		return nil, false, false
	}
	truncated := int64(len(buffer)) > maxBytes
	if truncated {
		return buffer[:maxBytes], true, true
	}
	return buffer, false, true
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (reader *replayReadCloser) Close() error {
	return reader.closer.Close()
}

func decodeCapturedBody(body []byte, mediaType string, truncated bool) (any, string) {
	if !truncated && isJSONMediaType(mediaType) {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err == nil {
			return value, "json"
		}
	}
	if !truncated && mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(body))
		if err == nil {
			return formValuesToFields(values), "form"
		}
	}
	return string(body), "text"
}

func formValuesToFields(values url.Values) Fields {
	fields := make(Fields, len(values))
	for key, values := range values {
		if len(values) == 1 {
			fields[key] = values[0]
			continue
		}
		items := make([]any, len(values))
		for i, value := range values {
			items[i] = value
		}
		fields[key] = items
	}
	return fields
}

func mediaTypeOf(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	}
	return strings.ToLower(mediaType)
}

func contentTypeAllowed(mediaType string, allowed []string) bool {
	if mediaType == "" {
		return false
	}
	if len(allowed) == 0 {
		return isJSONMediaType(mediaType)
	}
	for _, value := range allowed {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if value == mediaType {
			return true
		}
		if strings.HasSuffix(value, "/*") && strings.HasPrefix(mediaType, strings.TrimSuffix(value, "*")) {
			return true
		}
		if value == "application/*+json" && strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json") {
			return true
		}
	}
	return false
}

func isJSONMediaType(mediaType string) bool {
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}
