# Log Center SDK Go

SDK Go inicial para enviar eventos ao Log Center por batch assincrono.

## Uso basico

```go
client := logcenter.NewClient(logcenter.Config{
    Endpoint:    os.Getenv("LOGCENTER_ENDPOINT"),
    APIKey:      os.Getenv("LOGCENTER_API_KEY"),
    Environment: "production",
    Service:     "orders-api",
    Version:     "1.42.0",
})
defer client.Close(context.Background())

client.Info(ctx, "Order payload created", logcenter.Fields{"document_type": "order"})
client.Flush(context.Background())
```

## Exemplo completo local

O exemplo `examples/http-server` sobe um servidor HTTP instrumentado:

```bash
export LOGCENTER_ENDPOINT=<logcenter-endpoint>
export LOGCENTER_API_KEY=<logcenter-api-key>
go run ./examples/http-server
curl -i <example-server>/checkout
curl -i <example-server>/error
```

## HTTP middleware

```go
mux := http.NewServeMux()
mux.HandleFunc("/checkout", handler)
http.ListenAndServe(":8090", client.HTTPMiddleware()(mux))
```

O middleware gera ou propaga `request_id` e `trace_id`, registra `request_started`,
mede duracao, captura status HTTP e registra `request_finished`. O SDK nao captura
body de request/response por padrao.

## Resiliencia

- eventos entram em buffer interno limitado;
- envio remoto e feito em batch;
- `Flush` respeita timeout;
- falhas do Log Center incrementam contadores em `Stats`;
- quando o buffer enche, eventos debug/info sao descartados antes de erros/auditoria quando possivel.

## Testes

```bash
go test ./...
```

Os testes cobrem envio em batch, timeout, preservacao de eventos de erro quando o buffer enche, middleware HTTP, request started/finished, spans, logs, erros e auditoria no mesmo fluxo investigavel.
