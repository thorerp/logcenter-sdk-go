package logcenter

import "context"

func (client *Client) Debug(ctx context.Context, message string, fields Fields) bool {
	return client.log(ctx, LevelDebug, message, fields)
}

func (client *Client) Info(ctx context.Context, message string, fields Fields) bool {
	return client.log(ctx, LevelInfo, message, fields)
}

func (client *Client) Warn(ctx context.Context, message string, fields Fields) bool {
	return client.log(ctx, LevelWarn, message, fields)
}

func (client *Client) ErrorLog(ctx context.Context, message string, fields Fields) bool {
	return client.log(ctx, LevelError, message, fields)
}

func (client *Client) Fatal(ctx context.Context, message string, fields Fields) bool {
	return client.log(ctx, LevelFatal, message, fields)
}

func (client *Client) Log(ctx context.Context, level, message string, fields Fields) bool {
	return client.log(ctx, level, message, fields)
}

func (client *Client) log(ctx context.Context, level, message string, fields Fields) bool {
	request, _ := RequestFromContext(ctx)
	return client.enqueue(Event{
		EventType: EventTypeLogEvent,
		RequestID: request.RequestID,
		TraceID:   request.TraceID,
		SpanID:    request.SpanID,
		UserID:    request.UserID,
		TenantID:  request.TenantID,
		Operation: request.Operation,
		Level:     level,
		Message:   message,
		Metadata:  fields,
	})
}

func (client *Client) RecordError(ctx context.Context, err error, options ErrorOptions) bool {
	request, _ := RequestFromContext(ctx)
	message := options.Message
	if err != nil {
		message = err.Error()
	}
	severity := options.Severity
	if severity == "" {
		severity = SeverityError
	}

	return client.enqueue(Event{
		IdempotencyKey: options.IdempotencyKey,
		EventType:      EventTypeErrorEvent,
		Classification: options.Classification,
		RetentionHint:  options.RetentionHint,
		RequestID:      request.RequestID,
		TraceID:        request.TraceID,
		SpanID:         request.SpanID,
		UserID:         request.UserID,
		TenantID:       request.TenantID,
		Operation:      request.Operation,
		Severity:       severity,
		ErrorType:      options.Type,
		ErrorCode:      options.Code,
		ErrorMessage:   message,
		Fingerprint:    options.Fingerprint,
		StackTrace:     options.StackTrace,
		Metadata:       options.Metadata,
		Data:           options.Data,
	})
}

func (client *Client) Error(ctx context.Context, err error, options ErrorOptions) bool {
	return client.RecordError(ctx, err, options)
}

func (client *Client) Audit(ctx context.Context, audit AuditEvent) bool {
	request, _ := RequestFromContext(ctx)
	return client.enqueue(auditEventToEvent(request, audit))
}

func (client *Client) AuditSync(ctx context.Context, audit AuditEvent) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request, _ := RequestFromContext(ctx)
	return client.SendEventSync(ctx, auditEventToEvent(request, audit))
}

func auditEventToEvent(request RequestContext, audit AuditEvent) Event {
	operation := audit.Operation
	if operation == "" {
		operation = request.Operation
	}
	tenantID := audit.TenantID
	if tenantID == "" {
		tenantID = request.TenantID
	}
	var changes any
	if len(audit.Changes) > 0 {
		changes = audit.Changes
	}

	return Event{
		IdempotencyKey: audit.IdempotencyKey,
		EventType:      EventTypeAuditEvent,
		Classification: audit.Classification,
		RetentionHint:  audit.RetentionHint,
		RequestID:      request.RequestID,
		TraceID:        request.TraceID,
		ActorType:      audit.ActorType,
		ActorID:        audit.ActorID,
		TenantID:       tenantID,
		Operation:      operation,
		Action:         audit.Action,
		EntityType:     audit.EntityType,
		EntityID:       audit.EntityID,
		FieldName:      audit.FieldName,
		OldValue:       audit.OldValue,
		NewValue:       audit.NewValue,
		Changes:        changes,
		Reason:         audit.Reason,
		Metadata:       audit.Metadata,
		Data:           audit.Data,
	}
}
