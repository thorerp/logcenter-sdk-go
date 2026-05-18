# SDK Roadmap

Checklist de pendencias priorizadas para tornar o SDK reutilizavel em todos os
produtos, sem acoplar o core a um domínio especifico.

Status:

- `[ ]` pendente
- `[~]` em andamento
- `[x]` concluído

## Prioridades

1. `[x]` Client no-op/disabled
   Permitir SDK desligado sem acumular erro quando não houver `Endpoint`/`APIKey`.

2. `[x]` ConfigFromEnv()
   Ler `LOGCENTER_ENDPOINT`, `LOGCENTER_API_KEY`, `APP_ENV`, service, version,
   buffer, batch, retry e timeouts.

3. `[x]` Validação local de eventos
   Validar antes de enfileirar: campos obrigatorios por tipo, status, level,
   duração, audit, etc.

4. `[x]` Redaction configurável
   Permitir adicionar termos sensíveis extras por `Config`, sem alterar SDK.

5. `[x]` Redaction padrão ampliada
   Incluir termos genericos como `cpf`, `cnpj`, `email`, `phone`, `telefone`,
   `document`, `certificate`, `base64`, `file`, `pdf`, `xml`, `stripe`,
   `client_secret`, `secret_key` e `private_key`.

6. `[x]` Limites e truncamento
   Configurar teto para strings, `Metadata`, `Data`, valores de auditoria e
   evento inteiro antes de entrar no buffer.

7. `[x]` Middleware Gin nativo
   Usar `gin.Context`, `c.FullPath()`, `c.ClientIP()`, status real e enrichers
   para user/tenant.

8. `[x]` Panic recovery Gin
   Capturar panic com stack trace, request/trace/user/tenant e status 500.

9. `[x]` Operation genérico
   Criar `StartOperation`, `Operation.End` e helpers para operações que não são
   HTTP request.

10. `[x]` Operation events/steps
    Registrar eventos filhos com `entity_type`, `entity_id`, `action`,
    `description`, `status`, `metadata`, `data`.

11. `[x]` Captura opcional de body com allowlist
    Por rota/configuração, com limite de bytes e redaction forte. Nada
    automático global.

12. `[x]` Timeouts separados
    `SendTimeout`, `FlushTimeout`, `CloseTimeout`, mantendo compatibilidade com
    `Timeout`.

13. `[x]` Hooks de falha
    Callbacks para evento descartado, batch falhou, evento rejeitado, erro
    atualizado.

14. `[x]` Stats/health handler
    Handler HTTP ou funcao pronta para expor saúde do SDK e `Stats()`.

15. `[x]` RoundTripper instrumentado
    Instrumentar chamadas externas com span automático, latência, status e erro.

16. `[x]` Modo síncrono para eventos críticos
    `SendEventSync`/`AuditSync` com timeout curto.

17. `[x]` Idempotency key explícita
    `IdempotencyKey` no SDK mapeia para `event_id`, que já e a chave
    idempotente oficial no backend atual.

18. `[x]` Classification
    Campo/helper genérico no SDK: `operational`, `security`, `audit`,
    `critical`, `privacy`. O envelope já envia o campo; persistencia dedicada
    no backend pode ser adicionada depois.

19. `[x]` Retention hint
    Campo/helper genérico no SDK: `default`, `short`, `standard`, `long`,
    `audit`, `privacy`. O envelope já envia o campo; persistencia dedicada no
    backend pode ser adicionada depois.

20. `[x]` Outbox/fila durável
    `OutboxPath` opcional persiste eventos em JSONL antes do envio e remove
    eventos aceitos pela API usando `event_id`.

21. `[x]` Tamper evidence/hash chain
    `TamperEvidence` opcional calcula hash canônico, encadeia com hash anterior
    e grava a evidência em `metadata`, com HMAC e estado durável opcionais.
