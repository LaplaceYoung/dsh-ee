# Compatibilidade com Anthropic Messages (POST /v1/messages)

Além do gateway compatível com OpenAI, o dsh-ee oferece uma entrada de chamada de modelos no formato Anthropic Messages: o Claude Code, o SDK da Anthropic
ou qualquer cliente escrito conforme o protocolo Anthropic Messages podem chamar diretamente, com um **token de acesso unificado do dsh-ee**, os modelos já integrados à empresa.
A requisição é convertida em um OpenAI chat completions e entregue ao gateway existente, portanto permissões de modelo, cota mensal da conta/token, failover de upstream,
contabilização de uso e auditoria são idênticos aos do gateway OpenAI.

**Cobertura da v0**: `POST /v1/messages` sem streaming e com streaming (SSE), autenticação, envelope de mensagem e envelope de erro.
**Não faz**: execução de `count_tokens`, Batches, Files, cabeçalhos beta completos e ferramentas do lado do servidor (como `web_search`).

## 1. Conectar com o Claude Code

Use o dsh-ee como base URL da API da Anthropic e o **nome de modelo exposto** do dsh-ee (o `id` retornado por `GET /v1/models`,
não o nome do fornecedor upstream):

```sh
export ANTHROPIC_BASE_URL='https://dsh-ee.example.com'
export ANTHROPIC_API_KEY='ee_at_…'            # 客户端发 x-api-key
# 或者：export ANTHROPIC_AUTH_TOKEN='ee_at_…'   # 客户端发 Authorization: Bearer

# 关键：Claude Code 默认的 claude-* 模型名在 dsh-ee 里不存在，必须显式指定对外模型名
export ANTHROPIC_MODEL='company-chat'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='company-chat'
export ANTHROPIC_DEFAULT_SONNET_MODEL='company-chat'
export ANTHROPIC_DEFAULT_OPUS_MODEL='company-chat'
```

> O dsh-ee **não faz** mapeamento de nomes `claude-*`: o nome do modelo precisa estar entre os modelos disponíveis da conta/token, senão a resposta é 403.
> Camadas de compatibilidade com Anthropic de fornecedores como a DeepSeek mapeiam automaticamente os nomes claude para os modelos do próprio fornecedor; no dsh-ee
> os modelos são de vários fornecedores integrados pelo administrador, sem um destino único de mapeamento, por isso é exigido explicitamente o nome de modelo exposto.

O base URL tem duas formas equivalentes (o mesmo handler; comportamento, autenticação e envelope de erro idênticos):

| `ANTHROPIC_BASE_URL` | Requisição real do cliente | Descrição |
|---|---|---|
| `https://dsh-ee.example.com` | `POST /v1/messages` | Forma padrão de base URL da Anthropic |
| `https://dsh-ee.example.com/anthropic` | `POST /anthropic/v1/messages` | Compatível com o costume de base URL da Anthropic da DeepSeek (`<base>/anthropic`) |

Também é possível validar diretamente com curl:

```sh
curl -fsS "$BASE/v1/messages" \
  -H "x-api-key: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": [{"type": "text", "text": "ping"}]}]
  }'

# 同一个端点的别名（DeepSeek 风格 base URL：ANTHROPIC_BASE_URL=$BASE/anthropic）
curl -fsS "$BASE/anthropic/v1/messages" ...  # 参数与上面完全相同
```

## 2. Autenticação e permissões

- Credencial: `x-api-key: <token>` ou `Authorization: Bearer <token>`, equivalentes; quando os dois vierem juntos, `x-api-key` tem prioridade.
- Token: token de acesso unificado `ee_at_…` (também aceita os antigos `sk-ee-…` / `ee_npm_…`, com o mesmo comportamento do gateway OpenAI).
- Permissões: o token precisa do escopo `models` e a conta precisa da permissão «Chamar modelos» (`llm.use`); o modelo precisa estar entre os modelos disponíveis da conta e do token.
- Cota: a cota mensal da conta/token e sua expiração são as mesmas do gateway OpenAI; requisições recusadas não são encaminhadas ao upstream nem contabilizadas.
- Limite de taxa: compartilhado com `/v1/*`, com janela fixa por token (padrão de 60 por minuto, ajustável por `DSH_EE_RATE_LIMIT`).
  Requisições apenas com `x-api-key` também são contabilizadas por token, sem voltar a contar por IP; ao exceder, também retorna o envelope de erro da Anthropic com `Retry-After`.
- `anthropic-version` / `anthropic-beta`: no caminho de repasse nativo da DeepSeek, esses dois cabeçalhos são **repassados sem alteração** ao upstream (o suporte depende da DeepSeek; o dsh-ee não os interpreta); no caminho de conversão de fallback dos demais provedores, são ignorados sem erro.
- **Files API fora do escopo da v0**: mesmo com `anthropic-beta: files-api-2025-04-14` o dsh-ee não implementa Files; no caminho de conversão de fallback, a referência `image.source.type=file` retorna 400 (imagens aceitam apenas `base64` / `url`); no caminho de repasse nativo, blocos de arquivo/visão vão sem alteração para a DeepSeek; os endpoints de Files / Batches retornam 404.
- Descoberta de serviço: `models.anthropic_messages_endpoint` em `GET /.well-known/dsh-ee` informa o endereço deste endpoint,
  e `GET /v1/openapi.json` também traz a descrição de requisição/resposta/erros de `POST /v1/messages`.

## 3. Encaminhamento ao upstream: repasse nativo da DeepSeek / conversão de fallback dos demais provedores

O dsh-ee decide como a requisição chega ao upstream conforme as rotas candidatas do modelo exposto (a ordem das candidatas é a prioridade de `/admin/providers`):

| `provider.vendor` da primeira rota candidata | Forma de encaminhamento | O que o dsh-ee faz |
|---|---|---|
| `deepseek` | **Repasse nativo**: `POST <provider.base>/anthropic/v1/messages` | Somente autenticação, permissão de modelo, cota, limite de taxa e reescrita de `model` (nome exposto→nome upstream); `anthropic-version` / `anthropic-beta` repassados sem alteração; os demais campos do corpo (server tools, citations, safety, title, latest_reminder, blocos file/vision etc.) repassados sem alteração; resposta de sucesso JSON/SSE **repassada byte a byte**; o usage é lido em paralelo para contabilização (estimado quando ausente); erros do upstream também são repassados com o status/corpo originais. |
| Outros | **Conversão de fallback**: Anthropic Messages → OpenAI chat completions | Ver §4 «Conversão de requisição» e §5 «Resposta e streaming»; `anthropic-version` / `anthropic-beta` são ignorados; blocos não suportados retornam 400 ou são ignorados. |

- No repasse nativo o dsh-ee **não valida** parâmetros de geração como `max_tokens` / `temperature` (deixa para a DeepSeek) nem interpreta blocos de conteúdo; cota mensal, limite de taxa por token e permissões de modelo continuam sendo impostos pelo dsh-ee.
- **O failover só ocorre dentro do mesmo protocolo**: se a primeira rota é `deepseek`, só alterna entre outras rotas `deepseek`; caso contrário, só alterna entre rotas de conversão de outros provedores, **sem cruzar protocolos**; se tudo falhar, retorna o erro da última rota. Quando o mesmo modelo exposto tem várias rotas, a primeira (maior prioridade) define o protocolo.
- O repasse nativo também aceita o alias `/anthropic/v1/messages`; a autenticação `x-api-key` / Bearer é idêntica à da camada de conversão.
- O documento de descoberta publica essa política em `models.anthropic_messages_native_vendors` (padrão `["deepseek"]`) e `models.anthropic_messages_conversion_fallback` (`openai-chat`).

## 4. Conversão de requisição (caminho de conversão de fallback: Anthropic → OpenAI chat)

| Campo Anthropic | Tratamento |
|---|---|
| `model` | Valida a permissão pelo nome de modelo exposto do dsh-ee e troca pelo nome do modelo upstream no encaminhamento |
| `max_tokens` | Inteiro positivo obrigatório; limite por chamada igual ao do gateway OpenAI (32768); acima disso, 400 |
| `system` | String ou array de blocos de texto, mesclado em uma mensagem `system` do OpenAI |
| `messages[].content` | String sem alteração; blocos de conteúdo convertidos conforme a tabela abaixo |
| `temperature` / `top_p` | Repassados sem alteração (a faixa de `temperature` é definida pelo upstream) |
| `top_k` | O OpenAI chat não tem parâmetro correspondente; ignorado |
| `stop_sequences` | Array convertido em `stop` do OpenAI |
| `tools` | Ferramenta do cliente `{name, description, input_schema}` → `{type:"function", function:{name, description, parameters}}` |
| `tool_choice` | `auto`→`auto`, `any`→`required`, `none`→`none`, `tool`→function específica; `disable_parallel_tool_use`→`parallel_tool_calls=false` |
| `metadata.user_id` | Convertido em `user` do OpenAI, para o upstream limitar/isolar por usuário final |
| `thinking` | `enabled` / `adaptive` → parâmetro estendido `thinking:{"type":"enabled"}` do OpenAI; `disabled` → `thinking:{"type":"disabled"}`; `budget_tokens` ignorado |
| `reasoning_effort` / `output_config.effort` | Convertido em `reasoning_effort` do OpenAI; `none` desliga o raciocínio; `xhigh` / `max` / `ultra` normalizados para `high` (faixa `minimal/low/medium/high`) |
| `stream` | Encaminha em streaming; acrescenta automaticamente `stream_options.include_usage=true` para a contabilização |
| `container` / `mcp_servers` / `service_tier` / `top_k` / `cache_control` | Ignorados |

Blocos de conteúdo:

| Tipo de bloco | Posição | Conversão |
|---|---|---|
| `text` | user / assistant | Texto (vários blocos de texto na mesma mensagem são mesclados com quebra de linha) |
| `image` | user | `source.type=base64` / `url` → segmento de conteúdo `image_url` do OpenAI (o upstream precisa suportar visão) |
| `tool_use` | assistant | → `tool_calls` do OpenAI (`input` serializado como string JSON em `arguments`) |
| `tool_result` | user | → mensagem `role:"tool"` do OpenAI (`tool_use_id` → `tool_call_id`; conteúdo achatado em texto, imagens viram o marcador `[image]`) |
| `thinking` | assistant | → `reasoning_content` da mensagem assistant (upstreams como a DeepSeek exigem o retorno em turnos seguintes com tools) |
| `redacted_thinking` | assistant | Não é possível restaurar como texto; ignorado |
| `document` e outros blocos | — | 400 `invalid_request_error` (a v0 não suporta blocos de arquivo/documento) |

Ferramentas do lado do servidor (com `type` como `web_search_20250305`, `bash_20250124` e outras executadas no lado da Anthropic) são **ignoradas**,
e não fazem a requisição inteira falhar — assim os demais recursos do Claude Code continuam funcionando, mas ferramentas de busca na web/execução de código não têm efeito real.

## 5. Resposta e streaming (caminho de conversão de fallback)

A resposta sem streaming é uma `message` padrão da Anthropic: `id` (`msg_…`), `type:"message"`, `role:"assistant"`,
`model` (o nome de modelo exposto pedido pelo cliente), array de blocos `content`, `stop_reason`, `stop_sequence`, `usage`.

- Ordem de `content`: `thinking` (quando o upstream retorna `reasoning_content` / `reasoning`) → `text` → `tool_use`.
  O `signature` do bloco thinking é um valor opaco de preenchimento gerado pelo dsh-ee (o servidor não valida a assinatura); o cliente pode devolvê-lo sem alteração.
- `stop_reason`: com chamada de ferramenta → `tool_use`; `finish_reason=length` → `max_tokens`; `content_filter` → `refusal`;
  os demais → `end_turn`. O OpenAI não distingue o acionamento de `stop_sequences`, por isso `stop_sequence` não é retornado.
- `usage`: `output_tokens` recebe o `completion_tokens` do upstream; `input_tokens = prompt_tokens - acertos do cache`,
  `cache_read_input_tokens` recebe `prompt_tokens_details.cached_tokens` do upstream (0 quando o campo não existe),
  `cache_creation_input_tokens` é fixo em 0. A **contabilização** continua usando `prompt_tokens + completion_tokens` do upstream para a cota mensal.

Streaming com `Content-Type: text/event-stream`, e a sequência de eventos (igual à oficial da Anthropic):

```text
event: message_start          # message 骨架，usage.input_tokens 是估算值
event: content_block_start    # 依次出现 thinking / text / tool_use 块
event: content_block_delta    # thinking_delta / text_delta / input_json_delta / signature_delta
event: content_block_stop
event: message_delta          # delta.stop_reason + usage（最终用量，以这里为准）
event: message_stop
```

- Não envia o `data: [DONE]` do OpenAI; se a conexão for interrompida antes de `message_stop`, a resposta está incompleta.
- `message_start.usage.input_tokens` é uma estimativa do corpo da requisição (no início do stream o usage do upstream ainda não é conhecido);
  `input_tokens` / `output_tokens` em `message_delta.usage` são os valores finais.
- Quando o upstream não fornece usage em streaming (por exemplo, o cliente para no meio), o dsh-ee estima para a contabilização com base na requisição e no conteúdo já repassado, evitando que a cota deixe de ser registrada.

## 6. Erros

O corpo do erro tem o formato da Anthropic, com o status HTTP correspondendo a `error.type`:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"请求过于频繁，请稍后重试"}}
```

| HTTP | `error.type` | Gatilho |
|---|---|---|
| 400 | `invalid_request_error` | Corpo não é JSON, falta `model` / `max_tokens`, parâmetro fora da faixa, tipo de bloco não suportado |
| 401 | `authentication_error` | Token ausente ou inválido |
| 403 | `permission_error` | Token sem o escopo `models`, conta sem `llm.use`, modelo fora dos modelos disponíveis, cota expirada |
| 404 | `not_found_error` | Modelo exposto sem upstream disponível; endpoints de compatibilidade não implementados, como `count_tokens` |
| 413 | `request_too_large` | Corpo da requisição maior que 16 MiB |
| 429 | `rate_limit_error` | Cota mensal da conta/token esgotada, limite de taxa por token, upstream retornou 429 |
| 5xx | `api_error` / `overloaded_error` | Todas as rotas de upstream indisponíveis, upstream 5xx, resposta do upstream não interpretável |

`/v1/messages/count_tokens` primeiro autentica e depois retorna 404 `not_found_error` (não implementado na v0),
em vez de cair no envelope de erro OpenAI de fallback do `/v1/`.

No caminho de repasse nativo da DeepSeek, os erros retornados pelo upstream (a própria DeepSeek já usa o envelope de erro da Anthropic) são
encaminhados com o **status e o corpo originais**, sem reembalagem; os erros gerados pelo próprio dsh-ee (autenticação/permissão/cota/limite de taxa/modelo sem upstream/upstream inacessível) continuam usando a tabela acima.

## 7. Limitações conhecidas (v0)

- Implementa apenas `POST /v1/messages` (e o alias `POST /anthropic/v1/messages`); `count_tokens` (incluindo o alias),
  Batches, Files, `/v1/messages/batches` etc. retornam 404.
- Ferramentas do lado do servidor não são executadas (ver §3); `container`, `mcp_servers`, `service_tier`, `top_k` são ignorados.
- Não faz mapeamento de nomes `claude-*`: o cliente precisa configurar o nome do modelo como o nome exposto em `/v1/models`.
- O conteúdo de thinking é identificado pelos campos `reasoning_content` / `reasoning` do upstream; quando o upstream não retorna conteúdo de raciocínio,
  a resposta não tem bloco `thinking`. `budget_tokens` não tem efeito: a intensidade do raciocínio é controlada por `reasoning_effort` / `output_config.effort`.
- Imagens são convertidas em segmentos de conteúdo `image_url` do OpenAI, e se o modelo upstream aceita imagens é decisão do upstream; blocos de arquivo como `document` não são suportados.
- O acionamento de `stop_sequences` e o encerramento normal são ambos `finish_reason=stop` no upstream e não podem ser distinguidos; por isso retorna sempre `end_turn`.
- Assinatura não validada: o `signature` do bloco thinking é um valor de preenchimento; o servidor não verifica nem depende dele.
- No caminho de repasse nativo da DeepSeek, o dsh-ee não valida parâmetros de geração nem interpreta blocos de conteúdo; limites como `max_tokens` ficam a cargo da DeepSeek;
  erros do upstream são repassados sem alteração; as respostas do caminho nativo não passam pela camada de conversão Anthropic do dsh-ee.
- O failover não cruza protocolos: depois que a rota de maior prioridade define o protocolo, rotas de outro protocolo com menor prioridade não são tentadas.

## 8. Relacionados

- Gateway compatível com OpenAI, tokens e escopo: [Integração com o Harness](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- Manual completo do cliente de terceiros e tabela de erros: [l1-client.md](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- Índice de códigos de erro do gateway de modelos: [errors.md](https://github.com/hikariming/dsh-ee/blob/main/docs/errors.md)
- Implementação: `server/internal/api/anthropic.go`, `server/internal/api/anthropic_convert.go`
