# Conecta un Harness de terceros o tu propio cliente

## Ruta de cinco minutos (H1: solo `/mcp`)

Para que un Harness de terceros (Claude Code, Codex, un agente propio, un script) se conecte a dsh-ee, la ruta más corta es **H1: un token `ee_at_` + `/mcp`**. Si necesitas archivos de habilidad, una extensión local en el equipo o generar la configuración con un solo comando, sigue entonces la ruta H2 según la documentación del repositorio; la integración profunda exclusiva de DSH (H3) no está pensada para dependencias de terceros, así que no diseñes tu integración sobre H3.

| Paso | Qué hacer | Dónde mirar |
| --- | --- | --- |
| 1 | Crea un token `ee_at_` en «Mis tokens» y minimiza el scope según el uso | «Token y scope», más abajo |
| 2 | Lee el documento de descubrimiento anónimo `GET /.well-known/dsh-ee` y confirma `api_version`, el endpoint de código de dispositivo, las capacidades y los límites | «Descubrimiento y versión» |
| 3 | Con el scope `models`, llama a `/v1/models` y `/v1/chat/completions` | «Puerta de enlace de modelos» |
| 4 | Con el scope `connections`, conéctate a `/mcp` y haz `tools/list` de forma dinámica por sesión | «MCP» |
| 5 | Las herramientas de escritura pasan por confirmación en el servidor: obtén primero el enlace «confirma en la web» y, cuando el empleado acepte, vuelve a llamar con exactamente los mismos parámetros | «Operaciones de escritura» |

## Token y scope

Los tokens empiezan por `ee_at_` y corresponden al propio usuario; el scope es solo un límite superior y el servidor lo cruza en cada solicitud con los permisos de la cuenta (si la cuenta no tiene el permiso correspondiente, el token tampoco sirve).

| Uso | Scope necesario | Notas |
| --- | --- | --- |
| Llamar a modelos | `models` | `/v1/models` y `/v1/chat/completions`; la cuenta necesita además el permiso «Llamar a los modelos» |
| Usar las herramientas de conexiones de empresa, bases de conocimiento y datos de aplicaciones de `/mcp` | `connections` | La entrada `/mcp` se comprueba con `connections` |
| Usar las herramientas `ee_*` del Banco de trabajo en `/mcp` | `connections` + `agent` | El interruptor general del agente en la configuración del sistema también debe estar activado |
| Leer el catálogo de capacidades (opcional) | `hub` | Aplicaciones y su revision en `/v1/apps` y `/v1/capabilities` |

Un token equivale a una contraseña: léelo de una variable de entorno o del almacén de credenciales y no lo metas en repositorios, registros ni capturas de pantalla; emite uno por cada integrador y revócalo por persona.

## Descubrimiento y versión

El endpoint anónimo `GET /.well-known/dsh-ee` devuelve `api_version`, el endpoint de código de dispositivo, `scopes_supported`, las capacidades de modelos/MCP/aplicaciones, los límites de tamaño de solicitud y la política de deprecación. `GET /v1/capabilities` (Bearer opcional) devuelve los modelos, el número de herramientas y la revision visibles para el token actual; ambas respuestas llevan la cabecera `Dsh-EE-API-Version: 1`.

El inicio de sesión con código de dispositivo es una entrada pública opcional: `POST /api/device/code` devuelve `device_code` / `user_code`; después de que el empleado confirme en la página `/device`, se sondea `POST /api/device/token` según el `interval` devuelto. El código de dispositivo es válido durante 10 minutos, el token solo se puede obtener una vez, no hay refresh token y, si caduca, hay que repetir el proceso.

## Puerta de enlace de modelos (`/v1`)

Compatible con OpenAI; llamada mínima:

```sh
export BASE='https://<dsh-ee>'
export TOKEN='ee_at_…'

curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"

curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"<模型名>","messages":[{"role":"user","content":"你好"}]}'
```

El nombre del modelo se rige por lo que devuelve `/v1/models`; el streaming, los límites y los códigos de error se gestionan según las convenciones de compatibilidad con OpenAI.

## MCP (`/mcp`)

`/mcp` es Streamable HTTP + JSON-RPC 2.0: `POST /mcp` inicia la solicitud con `Accept: application/json, text/event-stream`; el `Mcp-Session-Id` que devuelve `initialize` se usa en las solicitudes siguientes.

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-harness","version":"1.0"}}}'
```

| Prefijo de herramienta | Origen |
| --- | --- |
| `<nombre-conexión>__<nombre-herramienta>` | Herramientas de las conexiones de empresa |
| `kb__*` | Herramientas de bases de conocimiento |
| `app__*` / `data__*` | Superficie de herramientas de las microaplicaciones |
| `ee_*` | Herramientas del Banco de trabajo (requieren el scope `agent` y el interruptor general del agente) |

Las herramientas visibles dependen de los permisos del dueño del token, así que pueden variar de un empleado a otro: **llama a `tools/list` de forma dinámica en cada sesión y no lo fijes en el código**. Si falta `annotations.readOnlyHint` en una herramienta, trátala como operación de escritura.

## Operaciones de escritura: confirmación en el servidor

Un Harness de terceros en el modo predeterminado (`non_dsh`) no puede ejecutar herramientas de escritura directamente: `tools/call` devuelve `isError: true` y un enlace «confirma en la web». Después de que el empleado revise los parámetros en la página `/confirm/<id>` y acepte, la operación solo se ejecuta si vuelves a llamar con **exactamente los mismos parámetros**; cada aceptación sirve una sola vez y es válida durante 10 minutos; si cambian los parámetros, hay que confirmar de nuevo. El cliente DSH puede dejar pasar la operación según su propio interruptor de confirmación, pero no des por hecho que un Harness de terceros tiene este atajo.

## Limitaciones

| Limitación | Descripción |
| --- | --- |
| H1 no cubre | El catálogo de habilidades, el Banco de trabajo, las páginas web de microaplicaciones y la instalación y actualización automática de plugins pertenecen a la integración profunda exclusiva de DSH; un Harness de terceros no debe darlos por disponibles |
| Tamaño de solicitud | `max_request_bytes` / `max_response_bytes` se rigen por el documento de descubrimiento |
| Ciclo de vida del token | No hay refresh token; al caducar se emite uno nuevo; el flujo de código de dispositivo solo permite obtener el token una vez |
| Scope y permisos | El scope es un límite superior; lo que manda es «permisos de la cuenta ∩ scope» |
| Estabilidad de la API | La forma existente de `/v1`, `/.well-known/dsh-ee` y `/mcp` no cambia; las capacidades nuevas se rigen por el documento de descubrimiento |
| Operaciones de escritura | Sin `readOnlyHint`, todo se trata como operación de escritura y pasa por el flujo de confirmación |

## Texto completo en el repositorio

Esta página del sitio es solo una introducción; los detalles de protocolo y endpoints están en la documentación del repositorio (los documentos marcados como «disponible tras la fusión» llegan con el PR de documentación correspondiente):

- [Acceso de un Harness de terceros (docs/harness-integration.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/harness-integration.md)
- [Acceso de clientes que no son DSH, nivel L1 (docs/l1-client.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- [Descubrimiento y capacidades (docs/l1-client.md §2, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md#2-%E5%8F%91%E7%8E%B0%E4%B8%8E%E8%83%BD%E5%8A%9B%E4%B8%AD%E7%AB%8B%E5%A5%91%E7%BA%A6-v1)
- [La página de Harness en la documentación de desarrollo (docs/dev/harness.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- [Resumen del protocolo de extensiones (docs/extension-protocol/README.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
