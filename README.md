# Log Center SDK Go

SDK Go para coletar requests, traces, spans, logs, erros e auditoria e enviar os
eventos ao Log Center por batch assincrono.

Este SDK cobre apenas a API de coleta/ingestao. Ele nao implementa recursos do
painel, como login, projetos, dashboard, consultas, usuarios ou gerenciamento de
API keys.

## Recursos

- Envio assincrono por batch para `POST /v1/ingest/batch`.
- Middleware HTTP para coleta automatica de `request_started` e `request_finished`.
- Request lifecycle manual para workers, jobs, filas e frameworks sem middleware.
- Spans com hierarquia, tipo, duracao, status, metadata e erros vinculados.
- Logs nos niveis `debug`, `info`, `warn`, `error` e `fatal`.
- Erros com codigo, tipo, severidade, fingerprint, stack trace e metadata.
- Auditoria com ator, tenant, entidade, campo, valores antigos/novos e changes.
- Evento cru via `SendEvent` para cobrir campos avancados do contrato de ingestao.
- Correlacao por `request_id`, `trace_id`, `span_id`, usuario, tenant e operacao.
- Redaction local para campos sensiveis antes do envio.
- Buffer interno, descarte controlado, flush, retry e estatisticas.
- Nenhum endpoint de API hardcoded no pacote.

## Instalacao

```bash
go get github.com/thorerp/logcenter-sdk-go
```

Requisito: Go 1.22 ou superior.

## Configuracao minima

Use variaveis de ambiente ou o mecanismo de configuracao do seu projeto. O SDK
nao define endpoint padrao.

```bash
export LOGCENTER_ENDPOINT=<logcenter-endpoint>
export LOGCENTER_API_KEY=<logcenter-api-key>
export APP_ENV=production
```

```go
package main

import (
	"context"
	"os"
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func main() {
	client := logcenter.NewClient(logcenter.Config{
		Endpoint:      os.Getenv("LOGCENTER_ENDPOINT"),
		APIKey:        os.Getenv("LOGCENTER_API_KEY"),
		Environment:   os.Getenv("APP_ENV"),
		Service:       "orders-api",
		Version:       "1.42.0",
		RetryAttempts: 2,
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer client.Close(shutdownCtx)

	ctx := context.Background()
	client.Info(ctx, "Order payload created", logcenter.Fields{
		"document_type": "order",
	})

	_ = client.Flush(context.Background())
}
```

## Config

| Campo | Obrigatorio | Padrao | Descricao |
| --- | --- | --- | --- |
| `Endpoint` | Sim | vazio | Base URL da API de ingestao. O SDK adiciona `/v1/ingest/batch`. |
| `APIKey` | Sim | vazio | Chave com escopo de ingestao. Enviada como `Authorization: Bearer ...`. |
| `Environment` | Nao | `development` | Ambiente do evento, por exemplo `production`, `staging` ou `development`. |
| `Service` | Nao | `go-service` | Nome logico do servico que esta emitindo eventos. |
| `Version` | Nao | vazio | Versao do servico, release ou commit. |
| `Timeout` | Nao | `2s` | Timeout por tentativa de envio remoto. |
| `BufferSize` | Nao | `1000` | Tamanho maximo do buffer local de eventos. |
| `BatchSize` | Nao | `100` | Quantidade maxima de eventos por batch. Limitado ao `BufferSize`. |
| `FlushInterval` | Nao | `1s` | Intervalo de envio automatico. |
| `RetryAttempts` | Nao | `0` | Tentativas extras para erros retryable. |
| `HTTPClient` | Nao | `http.DefaultClient` | Cliente HTTP customizado. |
| `Source` | Nao | sdk, sdk_version, runtime | Dados sobre a origem do batch. Nao inclui hostname por padrao. |

Se `Endpoint` ou `APIKey` estiverem vazios, o `Flush` falha com erro claro e os
contadores de falha sao atualizados.

## Conceitos

### Evento

Todo evento enviado ao Log Center passa pelo mesmo envelope de ingestao. O SDK
preenche automaticamente:

- `event_id`, quando vazio;
- `occurred_at`, quando vazio;
- `environment`, a partir do `Config`;
- `service`, a partir do `Config`;
- `service_version`, a partir do `Config.Version`.

### Correlacao

Use contexto para correlacionar logs, spans, erros e auditoria com request,
trace, usuario, tenant e operacao:

```go
ctx = logcenter.ContextWithRequestID(ctx, "req_custom")
ctx = logcenter.ContextWithTraceID(ctx, "trc_custom")
ctx = logcenter.ContextWithSpanID(ctx, "spn_custom")
ctx = logcenter.ContextWithUser(ctx, "user-id")
ctx = logcenter.ContextWithTenant(ctx, "tenant-id")
ctx = logcenter.ContextWithOperation(ctx, "process-order")
```

Tambem e possivel definir tudo de uma vez:

```go
ctx = logcenter.ContextWithRequest(ctx, logcenter.RequestContext{
	RequestID: "req_custom",
	TraceID:   "trc_custom",
	Operation: "process-order",
	UserID:    "user-id",
	TenantID:  "tenant-id",
})
```

### Metadata e Data

- `Metadata`: use para dados de diagnostico, filtros e contexto operacional.
- `Data`: use como payload estruturado complementar quando precisar preencher o
  campo `data` do contrato de ingestao.

Ambos passam por redaction antes de sair do processo.

## HTTP middleware

O middleware registra automaticamente:

- `request_started` antes do handler;
- `request_finished` depois do handler;
- `method`, `path`, `route_template`, `http_status`, `duration_ms`;
- `remote_addr` e `user_agent` em `metadata`;
- `request_id` a partir de `X-LogCenter-Request-Id` ou `X-Request-Id`;
- `trace_id` a partir de `X-LogCenter-Trace-Id`, `X-Trace-Id` ou `Traceparent`.

Uso basico:

```go
mux := http.NewServeMux()
mux.HandleFunc("/checkout", checkoutHandler)

handler := client.HTTPMiddleware(
	logcenter.HTTPRouteTemplate("/checkout"),
)(mux)

server := &http.Server{
	Addr:    ":8080",
	Handler: handler,
}
```

Com extracao de rota, usuario, tenant e metadata:

```go
handler := client.HTTPMiddleware(
	logcenter.HTTPRouteTemplateFunc(func(r *http.Request) string {
		return r.Pattern
	}),
	logcenter.HTTPUserIDFunc(func(r *http.Request) string {
		return r.Header.Get("X-User-ID")
	}),
	logcenter.HTTPTenantIDFunc(func(r *http.Request) string {
		return r.Header.Get("X-Tenant-ID")
	}),
	logcenter.HTTPMetadataFunc(func(r *http.Request) logcenter.Fields {
		return logcenter.Fields{
			"request_class": "public-api",
		}
	}),
	logcenter.HTTPDataFunc(func(r *http.Request) logcenter.Fields {
		return logcenter.Fields{
			"route_group": "checkout",
		}
	}),
)(mux)
```

Opcoes disponiveis:

| Opcao | Uso |
| --- | --- |
| `HTTPRouteTemplate(value)` | Define um template fixo para a rota. |
| `HTTPRouteTemplateFunc(fn)` | Resolve o template a partir do request. |
| `HTTPUserIDFunc(fn)` | Extrai o usuario do request. |
| `HTTPTenantIDFunc(fn)` | Extrai o tenant do request. |
| `HTTPMetadataFunc(fn)` | Adiciona metadata ao `request_started`. |
| `HTTPDataFunc(fn)` | Adiciona data ao `request_started`. |

O SDK nao captura body de request ou response automaticamente.

## Request manual

Use `StartRequest` quando a operacao nao for uma requisicao HTTP instrumentada
pelo middleware, por exemplo jobs, filas, consumers ou comandos internos.

```go
ctx, req := client.StartRequest(ctx, logcenter.RequestStartOptions{
	RequestID:     "req_custom",
	TraceID:       "trc_custom",
	UserID:        "user-id",
	TenantID:      "tenant-id",
	Operation:     "process-order",
	Method:        "POST",
	Path:          "/orders/123",
	RouteTemplate: "/orders/{id}",
	Metadata: logcenter.Fields{
		"queue": "orders",
	},
})

defer req.End(logcenter.RequestEndOptions{
	Status: logcenter.StatusSuccess,
	Metadata: logcenter.Fields{
		"result": "accepted",
	},
})
```

`RequestStartOptions`:

| Campo | Descricao |
| --- | --- |
| `RequestID` | ID do request. Gerado automaticamente se vazio. |
| `TraceID` | ID do trace. Gerado automaticamente se vazio. |
| `SpanID` | Span pai, quando existir. |
| `UserID` | Usuario relacionado. |
| `TenantID` | Tenant relacionado. |
| `Operation` | Nome da operacao. Se vazio, usa `Method + " " + Path`. |
| `Method` | Metodo ou tipo da operacao. |
| `Path` | Caminho ou identificador da operacao. |
| `RouteTemplate` | Template estavel da rota. Se vazio, usa `Path`. |
| `StartedAt` | Inicio customizado. Se vazio, usa agora. |
| `Metadata` | Metadata do evento `request_started`. |
| `Data` | Data do evento `request_started`. |

`RequestEndOptions`:

| Campo | Descricao |
| --- | --- |
| `Status` | Status final. Se vazio, usa `success`. |
| `HTTPStatus` | Status HTTP, quando existir. |
| `FinishedAt` | Termino customizado. Se vazio, usa agora. |
| `Metadata` | Metadata do evento `request_finished`. |
| `Data` | Data do evento `request_finished`. |

## Spans

Spans medem partes internas de uma operacao e ficam vinculados ao contexto
atual.

```go
ctx, span := client.StartSpan(ctx, "call-payment-provider",
	logcenter.SpanKind("client"),
	logcenter.SpanMetadata(logcenter.Fields{
		"provider": "payment-provider",
	}),
)
defer span.End(logcenter.StatusSuccess)

// trabalho instrumentado aqui
```

Ao registrar erro no span, o SDK tambem envia um `error_event` correlacionado:

```go
if err != nil {
	span.RecordError(err, "PAYMENT_PROVIDER_TIMEOUT")
	span.End(logcenter.StatusFailed)
	return err
}
```

Opcoes:

| Opcao | Descricao |
| --- | --- |
| `SpanKind(kind)` | Define o tipo do span. Padrao: `internal`. |
| `SpanMetadata(fields)` | Metadata do span. |
| `SpanData(fields)` | Data do span. |

## Logs

```go
client.Debug(ctx, "cache skipped", logcenter.Fields{"reason": "disabled"})
client.Info(ctx, "order created", logcenter.Fields{"order_id": "order-id"})
client.Warn(ctx, "retry scheduled", logcenter.Fields{"attempt": 2})
client.ErrorLog(ctx, "provider failed", logcenter.Fields{"provider": "payment"})
client.Fatal(ctx, "worker stopped", logcenter.Fields{"component": "consumer"})
```

Tambem e possivel usar um nivel dinamico:

```go
client.Log(ctx, logcenter.LevelInfo, "custom log", nil)
```

Niveis aceitos:

- `logcenter.LevelDebug`
- `logcenter.LevelInfo`
- `logcenter.LevelWarn`
- `logcenter.LevelError`
- `logcenter.LevelFatal`

## Erros

```go
err := doWork(ctx)
if err != nil {
	client.RecordError(ctx, err, logcenter.ErrorOptions{
		Code:        "WORK_FAILED",
		Type:        "work_failed",
		Severity:    logcenter.SeverityError,
		Fingerprint: "work_failed",
		StackTrace:  stackTrace,
		Metadata: logcenter.Fields{
			"worker": "orders",
		},
	})
}
```

Alias:

```go
client.Error(ctx, err, logcenter.ErrorOptions{Code: "WORK_FAILED"})
```

`ErrorOptions`:

| Campo | Descricao |
| --- | --- |
| `Code` | Codigo estavel do erro. |
| `Type` | Tipo/categoria do erro. |
| `Severity` | Severidade. Se vazio, usa `error`. |
| `Fingerprint` | Chave de agrupamento. |
| `StackTrace` | Stack trace como string. |
| `Message` | Mensagem manual. Se `err` nao for nil, usa `err.Error()`. |
| `Metadata` | Metadata do erro. |
| `Data` | Data do erro. |

## Auditoria

Use auditoria para registrar mudancas relevantes de dominio.

```go
client.Audit(ctx, logcenter.AuditEvent{
	ActorType:  "user",
	ActorID:    "user-id",
	TenantID:   "tenant-id",
	Action:     "order.approved",
	EntityType: "order",
	EntityID:   "order-id",
	Changes: []logcenter.Change{
		{Field: "status", OldValue: "pending", NewValue: "approved"},
	},
	Reason: "approved by operator",
	Metadata: logcenter.Fields{
		"source": "admin",
	},
})
```

`AuditEvent`:

| Campo | Descricao |
| --- | --- |
| `ActorType` | Tipo do ator, por exemplo `user`, `system` ou `service`. |
| `ActorID` | ID do ator. |
| `TenantID` | Tenant relacionado. |
| `Operation` | Operacao. Se vazio, usa a operacao do contexto. |
| `Action` | Acao auditada. Obrigatorio. |
| `EntityType` | Tipo da entidade. Obrigatorio. |
| `EntityID` | ID da entidade. Obrigatorio. |
| `FieldName` | Campo alterado, quando a auditoria for pontual. |
| `OldValue` | Valor antigo. |
| `NewValue` | Valor novo. |
| `Changes` | Lista de alteracoes. |
| `Reason` | Motivo da alteracao. |
| `Metadata` | Metadata da auditoria. |
| `Data` | Data da auditoria. |

O backend exige `Action`, `EntityType`, `EntityID` e ao menos `Changes` ou
`OldValue/NewValue`.

## Evento cru

Use `SendEvent` quando precisar preencher campos avancados diretamente. O SDK
ainda aplica defaults, contexto e redaction.

```go
client.SendEvent(ctx, logcenter.Event{
	EventType:     logcenter.EventTypeLogEvent,
	Level:         logcenter.LevelInfo,
	Message:       "custom event",
	RequestID:     "req_custom",
	TraceID:       "trc_custom",
	UserID:        "user-id",
	TenantID:      "tenant-id",
	Operation:     "custom-operation",
	Metadata:      logcenter.Fields{"key": "value"},
	Data:          logcenter.Fields{"payload_id": "payload-id"},
})
```

Tipos de evento aceitos:

| Event type | Helper recomendado | Campos principais |
| --- | --- | --- |
| `request_started` | `StartRequest` ou `HTTPMiddleware` | `request_id`, `trace_id`, `operation`, `started_at` |
| `request_finished` | `Request.End` ou `HTTPMiddleware` | `request_id`, `trace_id`, `status`, `finished_at`, `duration_ms` |
| `span` | `StartSpan` e `Span.End` | `trace_id`, `span_id`, `name`, `started_at`, `finished_at`, `duration_ms`, `status` |
| `log_event` | `Debug`, `Info`, `Warn`, `ErrorLog`, `Fatal`, `Log` | `level`, `message` |
| `error_event` | `RecordError` ou `Error` | `severity` e `error_code` ou `error_message` |
| `audit_event` | `Audit` | `action`, `entity_type`, `entity_id`, `changes` ou `old_value/new_value` |

## Status e constantes

Status aceitos em requests e spans:

- `logcenter.StatusStarted`
- `logcenter.StatusSuccess`
- `logcenter.StatusFailed`
- `logcenter.StatusTimeout`
- `logcenter.StatusCanceled`
- `logcenter.StatusIgnored`
- `logcenter.StatusRetrying`

Tipos de evento:

- `logcenter.EventTypeRequestStarted`
- `logcenter.EventTypeRequestFinished`
- `logcenter.EventTypeSpan`
- `logcenter.EventTypeLogEvent`
- `logcenter.EventTypeErrorEvent`
- `logcenter.EventTypeAuditEvent`

## Flush, Close e Stats

Eventos sao enfileirados localmente e enviados em background. Os metodos que
coletam eventos retornam `bool`:

- `true`: evento entrou no buffer;
- `false`: evento foi descartado ou o client ja foi fechado.

Force envio de tudo que esta no buffer:

```go
if err := client.Flush(ctx); err != nil {
	// registre ou trate a falha de envio
}
```

No shutdown da aplicacao, chame `Close` com timeout:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := client.Close(shutdownCtx); err != nil {
	// trate timeout/cancelamento de shutdown
}
```

Inspecione contadores:

```go
stats := client.Stats()
_ = stats.Queued
_ = stats.Dropped
_ = stats.SentEvents
_ = stats.FailedEvents
_ = stats.LastError
```

Campos de `Stats`:

| Campo | Descricao |
| --- | --- |
| `Queued` | Eventos aceitos no buffer local. |
| `Dropped` | Eventos descartados. |
| `SentEvents` | Eventos enviados com sucesso. |
| `SentBatches` | Batches enviados com sucesso. |
| `FailedEvents` | Eventos afetados por falha de envio. |
| `FailedBatches` | Batches com falha de envio. |
| `Accepted` | Eventos aceitos pela API. |
| `Duplicated` | Eventos marcados como duplicados pela API. |
| `Rejected` | Eventos rejeitados pela API. |
| `LastError` | Ultimo erro observado. |

## Redaction

Antes de enviar, o SDK mascara dados sensiveis em:

- `Metadata`;
- `Data`;
- `Message`;
- `ErrorMessage`;
- `StackTrace`;
- `Reason`;
- valores de auditoria (`Changes`, `OldValue`, `NewValue`).

Chaves com esses fragmentos sao mascaradas:

- `password`
- `senha`
- `token`
- `authorization`
- `cookie`
- `secret`
- `api_key`
- `apikey`
- `private_key`
- `pfx`
- `certificate_password`
- `cvv`

Strings com atribuicoes sensiveis e Bearer tokens tambem sao mascaradas.

Exemplo:

```go
client.Info(ctx, "created token=clear-value", logcenter.Fields{
	"api_key": "hidden",
	"safe":    "visible",
})
```

## Resiliencia e descarte

- Eventos entram em um buffer interno limitado por `BufferSize`.
- O envio remoto ocorre em batches de ate `BatchSize`.
- `FlushInterval` controla o envio periodico.
- `RetryAttempts` adiciona retentativas para erros retryable.
- Se o buffer estiver cheio, eventos `debug` e `info` sao descartados antes de
  eventos mais importantes, quando possivel.
- Falhas de envio incrementam `FailedBatches`, `FailedEvents` e `LastError`.

## Exemplo completo local

O diretorio `examples/http-server` contem um servidor HTTP instrumentado.

```bash
export LOGCENTER_ENDPOINT=<logcenter-endpoint>
export LOGCENTER_API_KEY=<logcenter-api-key>
go run ./examples/http-server
curl -i <example-server>/checkout
curl -i <example-server>/error
```

## Checklist de producao

- Configure `Endpoint` e `APIKey` fora do codigo fonte.
- Defina `Environment`, `Service` e `Version`.
- Use `HTTPRouteTemplate` ou `HTTPRouteTemplateFunc` para evitar alta
  cardinalidade em rotas.
- Propague `user_id`, `tenant_id`, `request_id` e `trace_id` quando existirem.
- Nao envie bodies completos por padrao.
- Coloque dados pesquisaveis em `Metadata`.
- Chame `Close` no shutdown da aplicacao.
- Monitore `Stats`, especialmente `Dropped`, `FailedEvents` e `LastError`.
- Ajuste `BufferSize`, `BatchSize`, `FlushInterval` e `RetryAttempts` conforme
  o volume do servico.

## Testes

```bash
go test ./...
```

Os testes cobrem envio em batch, timeout, endpoint obrigatorio, preservacao de
eventos de erro quando o buffer enche, middleware HTTP, request manual, spans,
logs, fatal, erros, auditoria, evento cru e redaction.
