# Compatibilidad con Anthropic Messages (POST /v1/messages)

Además de la puerta de enlace compatible con OpenAI, dsh-ee ofrece una entrada de llamadas a modelos en formato Anthropic Messages: Claude Code, el SDK de Anthropic o cualquier cliente escrito según el protocolo Anthropic Messages pueden llamar directamente a los modelos que la empresa ya tiene conectados usando el **token de acceso unificado de dsh-ee**.
La solicitud se convierte en una llamada OpenAI chat completions que se entrega a la puerta de enlace existente, de modo que los permisos de modelo, la cuota mensual de cuenta/token, la conmutación por error upstream, la contabilidad de uso y la auditoría son exactamente iguales que en la puerta de enlace OpenAI.

**Cobertura de v0**: `POST /v1/messages` en modo no streaming y streaming (SSE), autenticación, envoltorios de solicitud y respuesta y envoltorios de error.
**No incluye**: `count_tokens`, Batches, Files, cabeceras beta completas ni ejecución de herramientas del servidor (`web_search`, etc.).

## 1. Conecta Claude Code

Usa dsh-ee como base URL de la API de Anthropic y pon como nombre de modelo el **nombre de modelo público** de dsh-ee (el `id` que devuelve `GET /v1/models`, no el nombre del proveedor upstream):

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

> dsh-ee **no hace** el mapeo de nombres `claude-*`: el nombre del modelo debe estar entre los disponibles para la cuenta/token; si no, devuelve 403.
> Las capas de compatibilidad con Anthropic de proveedores como DeepSeek mapean automáticamente los nombres claude a sus propios modelos; los modelos de dsh-ee son modelos de varios proveedores conectados por el administrador y no tienen un único destino de mapeo, por eso se exige explícitamente el nombre de modelo público.

Hay dos formas equivalentes de escribir la base URL (el mismo handler; comportamiento, autenticación y envoltorios de error idénticos):

| `ANTHROPIC_BASE_URL` | Solicitud real del cliente | Notas |
|---|---|---|
| `https://dsh-ee.example.com` | `POST /v1/messages` | Forma estándar de base URL de Anthropic |
| `https://dsh-ee.example.com/anthropic` | `POST /anthropic/v1/messages` | Compatible con la costumbre de DeepSeek de usar `<base>/anthropic` como base URL de Anthropic |

También puedes verificarlo directamente con curl:

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

## 2. Autenticación y permisos

- Credenciales: `x-api-key: <token>` o `Authorization: Bearer <token>`; son equivalentes y, si se envían ambas, prevalece `x-api-key`.
- Token: token de acceso unificado `ee_at_…` (también se aceptan los antiguos `sk-ee-…` / `ee_npm_…`, con el mismo comportamiento que en la puerta de enlace OpenAI).
- Permisos: el token debe tener el scope `models`, la cuenta debe tener el permiso «Llamar a los modelos» (`llm.use`) y el modelo debe estar dentro del rango de modelos disponibles para la cuenta y el token.
- Cuota: la cuota mensual de cuenta/token y la caducidad de la cuota se comparten con la puerta de enlace OpenAI; las solicitudes rechazadas no se reenvían upstream ni se contabilizan.
- Límite de velocidad: comparte con `/v1/*` el límite de velocidad de ventana fija por token (60 por minuto por defecto, ajustable con `DSH_EE_RATE_LIMIT`). Las solicitudes que solo llevan `x-api-key` también consumen cuota por token y no vuelven a contarse por IP; al superar el límite se devuelve igualmente el envoltorio de error de Anthropic y `Retry-After`.
- `anthropic-version` / `anthropic-beta`: la ruta de paso nativo de DeepSeek **reenvía estas dos cabeceras tal cual** al upstream (que las soporte o no lo decide DeepSeek; dsh-ee no las interpreta); la ruta de conversión degradada para otros proveedores las ignora sin dar error.
- **La Files API no entra en el alcance de v0**: enviar `anthropic-beta: files-api-2025-04-14` tampoco hace que dsh-ee implemente Files; en la ruta de conversión degradada, una referencia a archivo `image.source.type=file` devuelve 400 (las imágenes solo admiten `base64` / `url`), y en la ruta de paso nativo los bloques de archivo/visión se entregan tal cual a DeepSeek; los endpoints Files / Batches devuelven 404.
- Descubrimiento: `models.anthropic_messages_endpoint` de `GET /.well-known/dsh-ee` indica la dirección de este endpoint, y `GET /v1/openapi.json` también contiene la descripción de solicitud/respuesta/errores de `POST /v1/messages`.

## 3. Reenvío upstream: paso nativo de DeepSeek / conversión degradada para otros proveedores

dsh-ee decide cómo llega la solicitud al upstream según la línea candidata del modelo público (el orden de candidatos es la prioridad de `/admin/providers`):

| `provider.vendor` de la primera línea candidata | Forma de reenvío | Qué hace dsh-ee |
|---|---|---|
| `deepseek` | **Paso nativo**: `POST <provider.base>/anthropic/v1/messages` | Solo hace autenticación, permisos de modelo, cuota, límite de velocidad y reescritura de `model` (nombre público→nombre upstream); `anthropic-version` / `anthropic-beta` se reenvían tal cual; el resto de campos del cuerpo (server tools, citations, safety, title, latest_reminder, bloques file/vision, etc.) se reenvían tal cual; la respuesta correcta JSON/SSE se reenvía **byte a byte**; se analiza el usage en paralelo para la contabilidad (si no hay usage, se estima); los errores del upstream también se reenvían con el estado y el cuerpo originales. |
| Otros | **Conversión degradada**: Anthropic Messages → OpenAI chat completions | Ver §4 «Conversión de solicitudes» y §5 «Respuesta y streaming»; `anthropic-version` / `anthropic-beta` se ignoran; los bloques no soportados devuelven 400 o se ignoran. |

- Con el paso nativo, dsh-ee **no valida** parámetros de generación como `max_tokens` / `temperature` (los entrega a DeepSeek) ni interpreta los bloques de contenido; la cuota mensual, el límite de velocidad por token y los permisos de modelo siguen siendo obligatorios en dsh-ee.
- **La conmutación por error solo ocurre dentro del mismo protocolo**: si la primera línea es `deepseek`, solo se cambia entre otras líneas `deepseek`; en caso contrario, solo se cambia entre líneas de conversión de otros proveedores, **nunca se cruza de protocolo**; si todo falla, se devuelve el error de la última línea. Cuando un mismo modelo público tiene varias líneas, la primera (la de mayor prioridad) determina el protocolo.
- El paso nativo también admite el alias `/anthropic/v1/messages`; la autenticación `x-api-key` / Bearer es idéntica a la de la capa de conversión.
- El documento de descubrimiento publica esta política en `models.anthropic_messages_native_vendors` (por defecto `["deepseek"]`) y `models.anthropic_messages_conversion_fallback` (`openai-chat`).

## 4. Conversión de solicitudes (ruta de conversión degradada: Anthropic → OpenAI chat)

| Campo de Anthropic | Tratamiento |
|---|---|
| `model` | Valida los permisos según el nombre de modelo público de dsh-ee y, al reenviar, lo sustituye por el nombre de modelo upstream |
| `max_tokens` | Entero positivo obligatorio; el límite por llamada es el mismo que en la puerta de enlace OpenAI (32768); si se supera, 400 |
| `system` | La cadena o el array de bloques de texto se combina en un único mensaje `system` de OpenAI |
| `messages[].content` | La cadena se deja tal cual; los bloques de contenido se convierten según la tabla siguiente |
| `temperature` / `top_p` | Se reenvían tal cual (el rango de `temperature` lo decide el upstream) |
| `top_k` | OpenAI chat no tiene parámetro equivalente; se ignora |
| `stop_sequences` | El array se convierte en `stop` de OpenAI |
| `tools` | Herramientas del cliente `{name, description, input_schema}` → `{type:"function", function:{name, description, parameters}}` |
| `tool_choice` | `auto`→`auto`, `any`→`required`, `none`→`none`, `tool`→función indicada; `disable_parallel_tool_use`→`parallel_tool_calls=false` |
| `metadata.user_id` | Se convierte en `user` de OpenAI para que el upstream pueda aplicar límite de velocidad o aislar por usuario final |
| `thinking` | `enabled` / `adaptive` → parámetro extendido de OpenAI `thinking:{"type":"enabled"}`; `disabled` → `thinking:{"type":"disabled"}`; `budget_tokens` se ignora |
| `reasoning_effort` / `output_config.effort` | Se convierte en `reasoning_effort` de OpenAI; `none` desactiva el razonamiento; `xhigh` / `max` / `ultra` se normalizan a `high` (valores `minimal/low/medium/high`) |
| `stream` | Reenvía en streaming; añade automáticamente `stream_options.include_usage=true` para la contabilidad |
| `container` / `mcp_servers` / `service_tier` / `top_k` / `cache_control` | Se ignoran |

Bloques de contenido:

| Tipo de bloque | Posición | Conversión |
|---|---|---|
| `text` | user / assistant | Texto (varios bloques `text` de un mismo mensaje se combinan con saltos de línea) |
| `image` | user | `source.type=base64` / `url` → segmento de contenido `image_url` de OpenAI (el upstream debe soportar visión) |
| `tool_use` | assistant | → `tool_calls` de OpenAI (el `input` se serializa como cadena JSON en `arguments`) |
| `tool_result` | user | → mensaje de OpenAI con `role:"tool"` (`tool_use_id` → `tool_call_id`; el contenido se aplana como texto y las imágenes se sustituyen por `[image]`) |
| `thinking` | assistant | → `reasoning_content` de ese mensaje assistant (upstreams como DeepSeek lo exigen al reenviar en turnos posteriores con tools) |
| `redacted_thinking` | assistant | No se puede reconstruir como texto; se ignora |
| `document` y otros bloques | — | 400 `invalid_request_error` (v0 no admite bloques de archivo/documento) |

Las herramientas del servidor (herramientas con `type` como `web_search_20250305`, `bash_20250124`, etc. que ejecuta el lado de Anthropic) se **ignoran** en lugar de hacer fallar toda la solicitud: así el resto de funciones de Claude Code siguen disponibles, pero las herramientas de búsqueda web o ejecución de código no llegan a activarse.

## 5. Respuesta y streaming (ruta de conversión degradada)

La respuesta no streaming es un `message` estándar de Anthropic: `id` (`msg_…`), `type:"message"`, `role:"assistant"`, `model` (el nombre de modelo público que pidió el cliente), array de bloques `content`, `stop_reason`, `stop_sequence` y `usage`.

- Orden de `content`: `thinking` (cuando el upstream devuelve `reasoning_content` / `reasoning`) → `text` → `tool_use`. La `signature` del bloque thinking es un valor opaco de relleno generado por dsh-ee (el servidor no valida la firma); el cliente puede devolverla tal cual.
- `stop_reason`: si hay llamadas a herramientas → `tool_use`; `finish_reason=length` → `max_tokens`; `content_filter` → `refusal`; en los demás casos → `end_turn`. OpenAI no distingue cuándo se ha alcanzado `stop_sequences`, así que no se devuelve `stop_sequence`.
- `usage`: `output_tokens` toma el `completion_tokens` del upstream; `input_tokens = prompt_tokens - aciertos de caché`, `cache_read_input_tokens` toma `prompt_tokens_details.cached_tokens` del upstream (0 si no existe ese campo) y `cache_creation_input_tokens` es siempre 0. **La contabilidad** sigue contando `prompt_tokens + completion_tokens` del upstream en la cuota mensual.

Streaming con `Content-Type: text/event-stream`; secuencia de eventos (igual que en Anthropic oficial):

```text
event: message_start          # message 骨架，usage.input_tokens 是估算值
event: content_block_start    # 依次出现 thinking / text / tool_use 块
event: content_block_delta    # thinking_delta / text_delta / input_json_delta / signature_delta
event: content_block_stop
event: message_delta          # delta.stop_reason + usage（最终用量，以这里为准）
event: message_stop
```

- No se envía el `data: [DONE]` de OpenAI; si la conexión se corta antes de `message_stop`, la respuesta está incompleta.
- `message_start.usage.input_tokens` es un valor estimado a partir del cuerpo de la solicitud (al empezar el stream aún no se conoce el usage del upstream); los `input_tokens` / `output_tokens` de `message_delta.usage` son los valores finales.
- Si el upstream no entrega usage de streaming (por ejemplo, porque el cliente se detiene a mitad), dsh-ee estima la contabilidad a partir de la solicitud y del contenido ya reenviado para no dejar cuota sin registrar.

## 6. Errores

El cuerpo de error tiene la forma de Anthropic y el código de estado HTTP se corresponde con `error.type`:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"请求过于频繁，请稍后重试"}}
```

| HTTP | `error.type` | Cuándo se produce |
|---|---|---|
| 400 | `invalid_request_error` | El cuerpo no es JSON, falta `model` / `max_tokens`, parámetros fuera de rango, tipo de bloque no soportado |
| 401 | `authentication_error` | Token ausente o no válido |
| 403 | `permission_error` | Al token le falta el scope `models`, a la cuenta le falta `llm.use`, el modelo no está en el rango o la cuota ha vencido |
| 404 | `not_found_error` | El modelo público no tiene upstream disponible; endpoints de compatibilidad no implementados, como `count_tokens` |
| 413 | `request_too_large` | El cuerpo supera los 16 MiB |
| 429 | `rate_limit_error` | Cuota mensual de cuenta/token agotada, límite de velocidad por token alcanzado o upstream que devuelve 429 |
| 5xx | `api_error` / `overloaded_error` | Ninguna línea upstream disponible, upstream 5xx, respuesta upstream no analizable |

`/v1/messages/count_tokens` autentica primero y después devuelve 404 `not_found_error` (v0 no lo implementa), en lugar de caer en el envoltorio de error OpenAI genérico de `/v1/`.

En la ruta de paso nativo de DeepSeek, los errores devueltos por el upstream (DeepSeek ya usa el envoltorio de error de Anthropic) se reenvían con el **código de estado y el cuerpo originales**, sin reempaquetar; los errores que genera dsh-ee (autenticación, permisos, cuota, límite de velocidad, modelo sin upstream o upstream inalcanzable) siguen usando la tabla anterior.

## 7. Limitaciones conocidas (v0)

- Solo se implementa `POST /v1/messages` (y el alias `POST /anthropic/v1/messages`); `count_tokens` (incluido su alias), Batches, Files, `/v1/messages/batches`, etc. devuelven 404.
- Las herramientas del servidor no se ejecutan (ver §3); `container`, `mcp_servers`, `service_tier` y `top_k` se ignoran.
- No se hace mapeo de nombres `claude-*`: el cliente debe configurar como nombre de modelo el nombre público de `/v1/models`.
- El contenido de thinking se reconoce por los campos `reasoning_content` / `reasoning` del upstream; si el upstream no devuelve contenido de razonamiento, la respuesta no llevará bloques `thinking`. `budget_tokens` no tiene efecto; la intensidad de razonamiento se controla con `reasoning_effort` / `output_config.effort`.
- Las imágenes se convierten en segmentos de contenido `image_url` de OpenAI; que el modelo upstream soporte imágenes lo decide el upstream; los bloques de archivo como `document` no se admiten.
- Si se alcanza `stop_sequences` o la respuesta termina de forma normal, el upstream devuelve `finish_reason=stop` en ambos casos y no se puede distinguir, así que se devuelve siempre `end_turn`.
- Las firmas no se validan: la `signature` del bloque thinking es un valor de relleno y el servidor no la verifica ni depende de ella.
- La ruta de paso nativo de DeepSeek no hace validación de parámetros de generación ni interpretación de bloques en dsh-ee; los límites como `max_tokens` los decide DeepSeek; los errores upstream se reenvían tal cual; las respuestas de la ruta nativa no pasan por la capa de conversión de Anthropic de dsh-ee.
- La conmutación por error no cruza de protocolo: una vez que la línea de mayor prioridad determina el protocolo, no se prueban líneas del otro protocolo con menor prioridad.

## 8. Relacionado

- Puerta de enlace compatible con OpenAI, tokens y scope: [Acceso de Harness (docs/dev/harness.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- Manual completo para clientes de terceros y tabla de errores: [l1-client.md](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- Índice de códigos de error de la puerta de enlace de modelos: [errors.md](https://github.com/hikariming/dsh-ee/blob/main/docs/errors.md)
- Implementación: `server/internal/api/anthropic.go`, `server/internal/api/anthropic_convert.go`
