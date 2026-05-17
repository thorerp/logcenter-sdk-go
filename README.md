# Log Center SDK Go

SDK Go para coletar requests, traces, spans, logs, erros e auditoria e enviar os
eventos ao Log Center por batch assincrono.

Este SDK cobre apenas a API de coleta/ingestao. Ele nao implementa recursos do
painel, como login, projetos, dashboard, consultas, usuarios ou gerenciamento de
API keys.

## Recursos

- Envio assincrono por batch para `POST /v1/ingest/batch`.
- Middleware HTTP para coleta automatica de `request_started` e `request_finished`.
- Middleware Gin nativo com rota, IP, status real, enrichers e recovery de panic.
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

Requisito: Go 1.26 ou superior.

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
	"time"

	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

func main() {
	config, err := logcenter.ConfigFromEnv()
	if err != nil {
		panic(err)
	}
	config.Service = "orders-api"
	config.Version = "1.42.0"
	config.RetryAttempts = 2

	client := logcenter.NewClient(config)

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

O jeito mais simples de carregar configuracao e `ConfigFromEnv()`:

```go
config, err := logcenter.ConfigFromEnv()
if err != nil {
	return err
}
client := logcenter.NewClient(config)
```

Variaveis lidas:

| Variavel | Campo | Formato |
| --- | --- | --- |
| `LOGCENTER_ENABLED` | `Enabled` | booleano: `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off`, `enabled`, `disabled` |
| `LOGCENTER_ENDPOINT` | `Endpoint` | string |
| `LOGCENTER_API_KEY` | `APIKey` | string |
| `LOGCENTER_ENVIRONMENT` | `Environment` | string |
| `APP_ENV` | `Environment` | fallback quando `LOGCENTER_ENVIRONMENT` nao existir |
| `LOGCENTER_SERVICE` | `Service` | string |
| `LOGCENTER_VERSION` | `Version` | string |
| `LOGCENTER_OUTBOX_PATH` | `OutboxPath` | caminho de arquivo JSONL |
| `LOGCENTER_TAMPER_EVIDENCE_ENABLED` | `TamperEvidence.Enabled` | booleano |
| `LOGCENTER_TAMPER_EVIDENCE_CHAIN_ID` | `TamperEvidence.ChainID` | string |
| `LOGCENTER_TAMPER_EVIDENCE_SECRET` | `TamperEvidence.Secret` | string |
| `LOGCENTER_TAMPER_EVIDENCE_STATE_PATH` | `TamperEvidence.StatePath` | caminho de arquivo JSON |
| `LOGCENTER_TAMPER_EVIDENCE_METADATA_KEY` | `TamperEvidence.MetadataKey` | string |
| `LOGCENTER_TAMPER_EVIDENCE_EVENT_TYPES` | `TamperEvidence.EventTypes` | lista separada por virgula |
| `LOGCENTER_TAMPER_EVIDENCE_CLASSIFICATIONS` | `TamperEvidence.Classifications` | lista separada por virgula |
| `LOGCENTER_TIMEOUT` | `Timeout` | duration Go, por exemplo `2s` ou `500ms` |
| `LOGCENTER_SEND_TIMEOUT` | `SendTimeout` | duration Go, por exemplo `2s` ou `500ms` |
| `LOGCENTER_FLUSH_TIMEOUT` | `FlushTimeout` | duration Go, por exemplo `5s` ou `500ms` |
| `LOGCENTER_CLOSE_TIMEOUT` | `CloseTimeout` | duration Go, por exemplo `5s` ou `500ms` |
| `LOGCENTER_BUFFER_SIZE` | `BufferSize` | inteiro |
| `LOGCENTER_BATCH_SIZE` | `BatchSize` | inteiro |
| `LOGCENTER_FLUSH_INTERVAL` | `FlushInterval` | duration Go, por exemplo `1s` ou `250ms` |
| `LOGCENTER_RETRY_ATTEMPTS` | `RetryAttempts` | inteiro |
| `LOGCENTER_SENSITIVE_KEY_FRAGMENTS` | `SensitiveKeyFragments` | lista separada por virgula |
| `LOGCENTER_MAX_STRING_BYTES` | `MaxStringBytes` | inteiro |
| `LOGCENTER_MAX_METADATA_BYTES` | `MaxMetadataBytes` | inteiro |
| `LOGCENTER_MAX_DATA_BYTES` | `MaxDataBytes` | inteiro |
| `LOGCENTER_MAX_AUDIT_VALUE_BYTES` | `MaxAuditValueBytes` | inteiro |
| `LOGCENTER_MAX_EVENT_BYTES` | `MaxEventBytes` | inteiro |

Tambem e possivel montar `Config` manualmente:

| Campo | Obrigatorio | Padrao | Descricao |
| --- | --- | --- | --- |
| `Enabled` | Nao | habilitado | Quando `logcenter.Bool(false)`, o client vira no-op: nao enfileira, nao envia e `Flush/Close` retornam nil. |
| `Endpoint` | Sim | vazio | Base URL da API de ingestao. O SDK adiciona `/v1/ingest/batch`. |
| `APIKey` | Sim | vazio | Chave com escopo de ingestao. Enviada como `Authorization: Bearer ...`. |
| `Environment` | Nao | `development` | Ambiente do evento, por exemplo `production`, `staging` ou `development`. |
| `Service` | Nao | `go-service` | Nome logico do servico que esta emitindo eventos. |
| `Version` | Nao | vazio | Versao do servico, release ou commit. |
| `Timeout` | Nao | `2s` | Timeout legado por tentativa de envio remoto. Usado como fallback de `SendTimeout`. |
| `SendTimeout` | Nao | valor de `Timeout` | Timeout por tentativa de envio remoto. |
| `FlushTimeout` | Nao | sem timeout proprio | Timeout maximo para `Flush`, alem do contexto recebido. |
| `CloseTimeout` | Nao | sem timeout proprio | Timeout maximo para `Close`, alem do contexto recebido. |
| `BufferSize` | Nao | `1000` | Tamanho maximo do buffer local de eventos. |
| `BatchSize` | Nao | `100` | Quantidade maxima de eventos por batch. Limitado ao `BufferSize`. |
| `FlushInterval` | Nao | `1s` | Intervalo de envio automatico. |
| `RetryAttempts` | Nao | `0` | Tentativas extras para erros retryable. |
| `HTTPClient` | Nao | `http.DefaultClient` | Cliente HTTP customizado. |
| `Source` | Nao | sdk, sdk_version, runtime | Dados sobre a origem do batch. Nao inclui hostname por padrao. |
| `Hooks` | Nao | vazio | Callbacks opcionais para descarte, falha de batch, rejeicao pela API e mudanca de ultimo erro. |
| `OutboxPath` | Nao | vazio | Caminho para outbox duravel JSONL. Quando vazio, usa apenas buffer em memoria. |
| `TamperEvidence` | Nao | desabilitado | Hash chain opcional gravado em metadata para evidenciar alteracao de eventos. |
| `SensitiveKeyFragments` | Nao | lista padrao | Fragmentos extras de nomes de campos que devem ser mascarados. |
| `MaxStringBytes` | Nao | `8192` | Teto por string antes de truncar. |
| `MaxMetadataBytes` | Nao | `65536` | Teto do JSON de `Metadata`. Limitado ao teto aceito pela API. |
| `MaxDataBytes` | Nao | `65536` | Teto do JSON de `Data`. Limitado ao teto aceito pela API. |
| `MaxAuditValueBytes` | Nao | `65536` | Teto do JSON de `Changes`, `OldValue` e `NewValue`. |
| `MaxEventBytes` | Nao | `262144` | Teto do evento inteiro depois de redaction/truncamento. |

Se `Endpoint` ou `APIKey` estiverem vazios, o `Flush` falha com erro claro e os
contadores de falha sao atualizados.

### Client no-op

Use o modo no-op em ambientes onde a coleta deve ficar desligada, sem gerar
erros por falta de `Endpoint` ou `APIKey`.

```go
client := logcenter.NewNoopClient()

_ = client.Info(ctx, "ignored", nil) // retorna false
_ = client.Flush(ctx)                // retorna nil
```

Ou via `Config`:

```go
client := logcenter.NewClient(logcenter.Config{
	Enabled: logcenter.Bool(false),
})
```

Quando desabilitado, o client nao inicia worker de envio, nao acumula erro em
`Stats` e nao faz chamadas HTTP.

## Conceitos

### Evento

Todo evento enviado ao Log Center passa pelo mesmo envelope de ingestao. O SDK
preenche automaticamente:

- `event_id`, quando vazio;
- `occurred_at`, quando vazio;
- `environment`, a partir do `Config`;
- `service`, a partir do `Config`;
- `service_version`, a partir do `Config.Version`.

Antes de entrar no buffer, o evento e validado localmente. Eventos invalidos
retornam `false`, incrementam `Stats.Dropped` e atualizam `Stats.LastError`.
Essa validacao evita enviar payloads que a API recusaria, por exemplo log sem
mensagem, auditoria sem entidade ou span sem duracao.

### Limites e truncamento

Antes de validar e enfileirar, o SDK aplica limites locais para reduzir o risco
de payloads acidentais, como strings enormes, blobs, documentos e dados
estruturados grandes demais.

O fluxo local e:

1. aplicar defaults do evento;
2. aplicar redaction;
3. truncar strings e campos estruturados;
4. validar o evento;
5. enfileirar.

Strings maiores que `MaxStringBytes` recebem o sufixo `...[TRUNCATED]`.
`Metadata`, `Data` e valores de auditoria maiores que seus respectivos tetos
sao substituidos por um placeholder de truncamento. Se, mesmo depois disso, o
evento inteiro ultrapassar `MaxEventBytes`, ele e descartado localmente,
retorna `false`, incrementa `Stats.Dropped` e atualiza `Stats.LastError`.

Exemplo:

```go
client := logcenter.NewClient(logcenter.Config{
	MaxStringBytes:     4096,
	MaxMetadataBytes:   32 * 1024,
	MaxDataBytes:       32 * 1024,
	MaxAuditValueBytes: 32 * 1024,
	MaxEventBytes:      128 * 1024,
})
```

### Idempotencia, classificacao e retencao

O backend atual usa `event_id` como chave idempotente por projeto. No SDK, use
`IdempotencyKey` quando quiser declarar essa chave de forma explicita; se
`EventID` estiver vazio, o SDK copia `IdempotencyKey` para `event_id` antes de
enviar.

```go
client.SendEvent(ctx, logcenter.Event{
	IdempotencyKey: "order-123:approved:v1",
	EventType:      logcenter.EventTypeLogEvent,
	Classification: logcenter.ClassificationCritical,
	RetentionHint:  logcenter.RetentionHintLong,
	Level:          logcenter.LevelInfo,
	Message:        "order approved",
})
```

`Classification` ajuda a diferenciar a natureza do evento:

- `logcenter.ClassificationOperational`
- `logcenter.ClassificationSecurity`
- `logcenter.ClassificationAudit`
- `logcenter.ClassificationCritical`
- `logcenter.ClassificationPrivacy`

`RetentionHint` e uma dica generica para politicas futuras de retencao:

- `logcenter.RetentionHintDefault`
- `logcenter.RetentionHintShort`
- `logcenter.RetentionHintStandard`
- `logcenter.RetentionHintLong`
- `logcenter.RetentionHintAudit`
- `logcenter.RetentionHintPrivacy`

Esses campos sao enviados no envelope do evento. Enquanto o backend nao tiver
colunas/filtros dedicados para `classification` e `retention_hint`, o efeito
garantido hoje e a idempotencia via `event_id`.

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

### Captura opcional de body

A captura de body e opt-in e deve ser aplicada apenas em rotas permitidas. O
middleware le ate o limite configurado, restaura o `Body` para o handler e
aplica redaction antes do envio.

```go
handler := client.HTTPMiddleware(
	logcenter.HTTPRequestBodyCaptureFunc(func(r *http.Request) bool {
		return r.Method == http.MethodPost && r.URL.Path == "/orders"
	}, 8*1024, "application/json"),
)(mux)
```

Tambem existe a forma simples para middlewares aplicados somente em rotas ja
permitidas:

```go
handler := client.HTTPMiddleware(
	logcenter.HTTPRequestBodyCapture(8*1024, "application/json"),
)(mux)
```

Quando capturado, o body entra em `Data["request_body"]` com `content_type`,
`encoding`, `size_bytes`, `max_bytes`, `truncated` e `value`. JSON e form-urlencoded
sao decodificados para estrutura; outros formatos permitidos entram como texto.
Se `contentTypes` ficar vazio, apenas JSON e tipos `+json` sao aceitos.

## Gin middleware e recovery

Para projetos Gin, use o pacote `integrations/gin`. Ele usa `gin.Context`,
`c.FullPath()` para template de rota, `c.ClientIP()` para IP de origem e o
status real gravado no `ResponseWriter`.

```go
import (
	"github.com/gin-gonic/gin"

	logcentergin "github.com/thorerp/logcenter-sdk-go/integrations/gin"
	"github.com/thorerp/logcenter-sdk-go/logcenter"
)

router := gin.New()

router.Use(logcentergin.Middleware(client))
router.Use(func(c *gin.Context) {
	userID, tenantID := authFromContext(c)
	logcentergin.SetIdentity(c, userID, tenantID)
	c.Next()
})
router.Use(logcentergin.Recovery(client))

router.POST("/orders/:id/approve", approveOrder)
```

A ordem recomendada e:

1. `logcentergin.Middleware(client)`, para iniciar o request;
2. middleware de autenticacao/autorizacao, para enriquecer com usuario e tenant;
3. `logcentergin.Recovery(client)`, para capturar panic com contexto completo;
4. handlers da aplicacao.

Tambem e possivel customizar rota, operacao, usuario, tenant, metadata e data:

```go
router.Use(logcentergin.Middleware(client,
	logcentergin.RequestBodyCaptureFunc(func(c *gin.Context) bool {
		return c.FullPath() == "/orders/:id/approve"
	}, 8*1024, "application/json"),
	logcentergin.OperationFunc(func(c *gin.Context) string {
		return c.Request.Method + " " + c.FullPath()
	}),
	logcentergin.UserIDFunc(func(c *gin.Context) string {
		return c.GetHeader("X-User-ID")
	}),
	logcentergin.TenantIDFunc(func(c *gin.Context) string {
		return c.GetHeader("X-Tenant-ID")
	}),
	logcentergin.MetadataFunc(func(c *gin.Context) logcenter.Fields {
		return logcenter.Fields{"route_group": "orders"}
	}),
))
```

Depois do auth, use os helpers abaixo para enriquecer logs, erros, spans e o
`request_finished` gerado pelo middleware:

```go
logcentergin.SetUserID(c, "user-id")
logcentergin.SetTenantID(c, "tenant-id")
logcentergin.SetIdentity(c, "user-id", "tenant-id")
logcentergin.SetOperation(c, "approve-order")
```

O recovery registra um `error_event` com `error_type=panic`, stack trace,
request/trace/user/tenant e encerra a resposta com HTTP 500 quando ela ainda
nao foi escrita.

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

## Operacoes genericas

Use `StartOperation` quando quiser medir uma operacao de negocio ou trabalho em
background sem tratar aquilo como rota HTTP. O SDK registra o inicio/fim usando
o mesmo lifecycle de request, preservando `request_id`, `trace_id`, usuario,
tenant, duracao e status.

```go
ctx, operation := client.StartOperation(ctx, "process-order", logcenter.OperationStartOptions{
	Kind:     "job",
	Metadata: logcenter.Fields{"queue": "orders"},
})
defer func() {
	operation.EndWithContext(ctx, logcenter.OperationEndOptions{
		Status: logcenter.StatusSuccess,
	})
}()

ctx = logcenter.ContextWithUser(ctx, "user-id")
ctx = logcenter.ContextWithTenant(ctx, "tenant-id")

operation.StepWithContext(ctx, logcenter.OperationEvent{
	Action:      "order.validated",
	EntityType:  "order",
	EntityID:    "order-id",
	Description: "order validated",
	Status:      logcenter.StatusSuccess,
	Metadata:    logcenter.Fields{"step": "validate"},
})
```

`OperationEvent` envia um `log_event` correlacionado com a operacao atual. Ele
serve para steps filhos que nao sao auditoria formal, mas precisam de contexto
pesquisavel:

| Campo | Descricao |
| --- | --- |
| `Action` | Acao estavel, por exemplo `order.validated`. |
| `EntityType` | Tipo da entidade relacionada. |
| `EntityID` | ID da entidade relacionada. |
| `Description` | Mensagem humana do step. Se vazio, usa `Action`. |
| `Status` | Status do step, quando fizer sentido. |
| `Level` | Nivel do log. Padrao: `info`. |
| `Kind` | Tipo do evento filho. Padrao: `operation_event`. |
| `Metadata` | Campos pesquisaveis do step. |
| `Data` | Payload complementar do step. |

Quando ja houver um contexto correlacionado e voce nao precisar manter um
handle de operacao, use `OperationEvent` direto:

```go
client.OperationEvent(ctx, logcenter.OperationEvent{
	Action:      "order.queued",
	EntityType:  "order",
	EntityID:    "order-id",
	Description: "order queued",
})
```

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

## RoundTripper instrumentado

Use `RoundTripper` para instrumentar chamadas HTTP externas feitas por
`http.Client`. Ele registra um `span` do tipo `client` com latencia, status
HTTP, host/path em metadata e, quando a chamada falha, um `error_event`
correlacionado ao mesmo span.

```go
httpClient := &http.Client{
	Transport: client.RoundTripper(http.DefaultTransport,
		logcenter.RoundTripperErrorCode("UPSTREAM_FAILED"),
		logcenter.RoundTripperMetadataFunc(func(req *http.Request, resp *http.Response, err error) logcenter.Fields {
			return logcenter.Fields{"peer_service": "billing-api"}
		}),
	),
}

req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
if err != nil {
	return err
}
resp, err := httpClient.Do(req)
```

Opcoes disponiveis:

| Opcao | Uso |
| --- | --- |
| `RoundTripperSpanName(value)` | Define um nome fixo para o span. |
| `RoundTripperSpanNameFunc(fn)` | Resolve o nome do span a partir do request. |
| `RoundTripperMetadataFunc(fn)` | Adiciona metadata com acesso a request, response e erro. |
| `RoundTripperDataFunc(fn)` | Adiciona data com acesso a request, response e erro. |
| `RoundTripperErrorCode(value)` | Codigo usado no span e no `error_event` quando `RoundTrip` falha. |

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
	IdempotencyKey: "order-id:approved:v1",
	Classification: logcenter.ClassificationAudit,
	RetentionHint:  logcenter.RetentionHintAudit,
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
	IdempotencyKey: "custom-event-id",
	EventType:      logcenter.EventTypeLogEvent,
	Classification: logcenter.ClassificationOperational,
	RetentionHint:  logcenter.RetentionHintStandard,
	Level:          logcenter.LevelInfo,
	Message:        "custom event",
	RequestID:      "req_custom",
	TraceID:        "trc_custom",
	UserID:         "user-id",
	TenantID:       "tenant-id",
	Operation:      "custom-operation",
	Metadata:       logcenter.Fields{"key": "value"},
	Data:           logcenter.Fields{"payload_id": "payload-id"},
})
```

### Envio sincrono

Use `SendEventSync` quando um evento critico precisar ser enviado antes de
continuar o fluxo. Ele envia direto para a API, sem passar pelo buffer local, e
retorna erro em falha de envio ou rejeicao pela API.

```go
ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
defer cancel()

err := client.SendEventSync(ctx, logcenter.Event{
	EventType: logcenter.EventTypeLogEvent,
	Level:     logcenter.LevelInfo,
	Message:   "critical event",
})
if err != nil {
	return err
}
```

Para auditoria:

```go
err := client.AuditSync(ctx, logcenter.AuditEvent{
	ActorType:  "user",
	ActorID:    "user-id",
	Action:     "order.approved",
	EntityType: "order",
	EntityID:   "order-id",
	Changes: []logcenter.Change{
		{Field: "status", OldValue: "pending", NewValue: "approved"},
	},
})
```

Em ambientes com o client desabilitado/no-op, os metodos sincronos retornam nil.

Tipos de evento aceitos:

| Event type | Helper recomendado | Campos principais |
| --- | --- | --- |
| `request_started` | `StartRequest`, `StartOperation` ou `HTTPMiddleware` | `request_id`, `trace_id`, `operation`, `started_at` |
| `request_finished` | `Request.End`, `Operation.End` ou `HTTPMiddleware` | `request_id`, `trace_id`, `status`, `finished_at`, `duration_ms` |
| `span` | `StartSpan` e `Span.End` | `trace_id`, `span_id`, `name`, `started_at`, `finished_at`, `duration_ms`, `status` |
| `log_event` | `Debug`, `Info`, `Warn`, `ErrorLog`, `Fatal`, `Log`, `OperationEvent` | `level`, `message` |
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

### Health HTTP

Use `Health()` ou `HealthHandler()` para expor o estado local do SDK em
healthchecks internos ou endpoints de diagnostico.

```go
mux.Handle("/health/logcenter", client.HealthHandler())
```

Por padrao, o handler responde HTTP 200 mesmo quando o status do SDK estiver
`degraded` ou `disabled`, para nao derrubar a saude da aplicacao por uma falha
de telemetria. Se quiser refletir o status no HTTP:

```go
mux.Handle("/health/logcenter", client.HealthHandler(logcenter.HealthHandlerOptions{
	DegradedStatusCode: http.StatusServiceUnavailable,
	DisabledStatusCode: http.StatusServiceUnavailable,
}))
```

O JSON inclui `status`, `enabled`, `sdk_version`, `runtime`, `service`,
`environment`, `queue_length`, `checked_at` e `stats`. Os status possiveis sao:

- `ok`: client habilitado sem falhas observadas;
- `degraded`: houve drop, falha de envio, rejeicao pela API ou `LastError`;
- `disabled`: client no-op/desabilitado.

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
- `secret_key`
- `client_secret`
- `api_key`
- `apikey`
- `private_key`
- `pfx`
- `certificate`
- `certificate_password`
- `certificado`
- `senha_certificado`
- `base64`
- `file`
- `arquivo`
- `document`
- `pdf`
- `xml`
- `logo`
- `stripe`
- `csrt`
- `chave_acesso`
- `cpf`
- `cnpj`
- `email`
- `phone`
- `telefone`
- `cvv`

Strings com atribuicoes sensiveis e Bearer tokens tambem sao mascaradas.

Para adicionar fragmentos sensiveis do seu produto:

```go
client := logcenter.NewClient(logcenter.Config{
	SensitiveKeyFragments: []string{"customer_code", "session_id"},
})
```

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
- `OutboxPath`, quando configurado, persiste eventos em arquivo antes do envio
  e remove cada evento quando a API aceita o batch.
- Se o buffer estiver cheio, eventos `debug` e `info` sao descartados antes de
  eventos mais importantes, quando possivel.
- Falhas de envio incrementam `FailedBatches`, `FailedEvents` e `LastError`.

### Outbox duravel

Configure `OutboxPath` para reduzir perda de eventos em queda de processo ou
indisponibilidade temporaria do Log Center. O arquivo e JSONL e usa `event_id`
para remover eventos aceitos.

```go
client := logcenter.NewClient(logcenter.Config{
	Endpoint:   endpoint,
	APIKey:     apiKey,
	OutboxPath: "/var/lib/my-service/logcenter-outbox.jsonl",
})
```

Quando o envio falha, o evento permanece no outbox e sera tentado novamente em
`Flush`, no envio periodico ou no `Close`. Como a API trata `event_id` de forma
idempotente por projeto, reenvios sao seguros.

### Tamper evidence

Use `TamperEvidence` quando precisar de uma cadeia de hashes local para eventos
criticos. O SDK calcula um hash canonico do evento redigido e limitado, encadeia
com o hash anterior e grava a evidencia em `Metadata["logcenter_integrity"]`.
Como `metadata` ja e persistido pelo backend atual, isso funciona sem migration.

```go
client := logcenter.NewClient(logcenter.Config{
	TamperEvidence: logcenter.TamperEvidenceConfig{
		Enabled:   true,
		ChainID:   "orders-critical",
		Secret:    os.Getenv("LOGCENTER_TAMPER_EVIDENCE_SECRET"),
		StatePath: "/var/lib/my-service/logcenter-chain.json",
		EventTypes: []string{
			logcenter.EventTypeAuditEvent,
			logcenter.EventTypeErrorEvent,
		},
		Classifications: []string{
			logcenter.ClassificationCritical,
			logcenter.ClassificationAudit,
		},
	},
})
```

Campos gravados em metadata:

- `version`
- `algorithm`: `sha256` ou `hmac-sha256`
- `chain_id`
- `sequence`
- `previous_hash`
- `canonical_hash`
- `hash`

`Secret` e opcional; quando informado, o hash da cadeia usa HMAC-SHA256. Use
`StatePath` para manter `sequence` e `previous_hash` entre reinicios. Sem
`StatePath`, a cadeia continua util dentro do processo atual, mas reinicia junto
com a aplicacao.

### Hooks de falha

Use hooks quando a aplicacao precisar observar descartes e falhas sem depender
de polling em `Stats()`.

```go
client := logcenter.NewClient(logcenter.Config{
	Hooks: logcenter.Hooks{
		OnEventDropped: func(drop logcenter.EventDrop) {
			// drop.Event, drop.Reason, drop.Err
		},
		OnBatchFailed: func(failure logcenter.BatchFailure) {
			// failure.Events, failure.EventCount, failure.Err
		},
		OnEventRejected: func(rejection logcenter.EventRejection) {
			// rejection.Event, rejection.Error
		},
		OnErrorChanged: func(change logcenter.ErrorChange) {
			// change.LastError, change.Err
		},
	},
})
```

Os hooks sao chamados de forma sincrona no ponto da falha e panics dentro do
callback sao recuperados pelo SDK.

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
