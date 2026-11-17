# Escribe una extensión de dsh-ee: manifiesto, comprobaciones y llamadas

## Cuatro formas

Las extensiones de dsh-ee conectan sistemas de la empresa. Primero elige la forma: las tres primeras se entregan con un manifiesto; la cuarta es una integración oficial que implementa el propio dsh-ee y para la que el autor no escribe manifiesto.

| Forma | `kind` del manifiesto | Dónde se ejecuta | Entregable | Cómo la llama dsh-ee |
| --- | --- | --- | --- | --- |
| Declarativa | `declarative` | Envía HTTP desde el proceso de dsh-ee | Manifiesto + documento OpenAPI (zip) | Genera herramientas a partir de OpenAPI y solicita directamente al sistema de origen |
| De servicio | `service` | Servicio HTTP desplegado por la propia empresa | Un servicio que implementa el manifiesto y la API acordada | Obtiene `/.well-known/dsh-ee-extension.json`; las herramientas van por `POST /mcp` |
| Local | `local` | Paquete npm en el ordenador del empleado | Paquete npm + manifiesto | El agente se conecta directamente al MCP local; antes de escribir pregunta a dsh-ee para confirmar y después de llamar informa un resumen |
| Integrada | (sin manifiesto) | Go dentro del proceso de dsh-ee | Se actualiza con dsh-ee | Usa la misma interfaz interna que las extensiones de servicio, sin pasar por la red (por ejemplo, la fuente de conocimiento de documentos de Feishu) |

El schema del manifiesto solo reconoce tres valores de `kind`: `declarative` / `service` / `local`; la forma «integrada» la implementa el propio dsh-ee y no se puede entregar mediante manifiesto. El servidor no ejecuta código de terceros: en la declarativa, dsh-ee envía las solicitudes; la extensión de servicio vive en el servicio propio de la empresa; la local, en el ordenador del empleado.

## Crea un esqueleto

```bash
# 声明式：不写代码，有 OpenAPI 就能接
dsh-ee extension init ./my-ext --id acme/my-ext --kind declarative

# 服务式：要写逻辑，企业自己部署
dsh-ee extension init ./my-service --id acme/my-service --kind service
```

`init` debe indicar la identidad con `--id autor/nombre`; `--kind` admite `declarative` (predeterminado) o `service`. Para las extensiones locales, copia la plantilla o el ejemplo de [`extensions/official/legacy-local`](https://github.com/hikariming/dsh-ee/blob/main/extensions/official/legacy-local) del repositorio. El directorio generado incluye un `AGENTS.md`; si lo abres con un agente, sabrá cuál es el siguiente paso.

| Comando | Uso | Qué hace en la máquina |
| --- | --- | --- |
| `dsh-ee extension init` | Copia una extensión desde un ejemplo | Escribe archivos |
| `dsh-ee schema list` / `schema get manifest` | Consulta sin conexión especificaciones como el manifiesto y los códigos de error | Solo lectura |
| `dsh-ee extension check` | Comprobación estática del manifiesto, la documentación de la API, las marcas de solo lectura y la forma de las credenciales | Comprobación de solo lectura; no ejecuta código |
| `dsh-ee extension test` | Arranca la extensión de servicio y la prueba punto por punto según el protocolo | Inicia un proceso local |
| `dsh-ee extension dev` | Arranca un dsh-ee simulado (sin base de datos) | Inicia un proceso local |
| `dsh-ee extension call` | Llama una vez a una herramienta de la extensión como un empleado concreto | Diagnóstico de solo lectura |
| `dsh-ee extension pack` | Empaqueta una extensión **declarativa** en un zip y calcula la suma de comprobación | Escribe un zip |
| `dsh-ee extension report` | Genera la nota de entrega para el administrador | Escribe un archivo |

Cuando `check` tiene errores, el código de salida es 1 y cada resultado incluye `code`, `severity`, `where`, `message` y `fix`. Todos estos comandos se ejecutan en la máquina o el CI del autor; el servidor de dsh-ee no ejecuta código de terceros ni ejecuta los checkers en la web.

## Ejemplo mínimo de manifiesto

El siguiente manifiesto coincide con la forma que genera `dsh-ee extension init --kind declarative`; si lo acompañas de un `openapi.yaml` cubierto por `servers`, el resultado de `dsh-ee extension check` es 0 errores y 0 avisos. Los campos se rigen por `dsh-ee schema get manifest`; el manifiesto usa un schema estricto y escribir un campo que no esté en la especificación también da error.

```json
{
  "id": "acme/oa",
  "version": "0.1.0",
  "name": "Acme OA",
  "description": "查询和提交 Acme OA 的审批单。",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": ["https://oa.acme.internal"],
  "credential": { "mode": "ee_jwt" },
  "tools": [
    {
      "name": "list_items",
      "readOnly": true,
      "description": "列出当前员工的条目",
      "keep": ["items.id", "items.title", "total"]
    },
    {
      "name": "create_item",
      "description": "新建一个条目。这是写操作，调用前要员工确认。"
    }
  ]
}
```

Algunos puntos en los que es fácil equivocarse:

- `id` debe escribirse como «autor/nombre», solo con minúsculas, dígitos y guiones; `version` es una versión semántica estricta.
- La declarativa debe tener `openapi` (ruta relativa dentro del directorio de la extensión) y al menos un `servers`; el `servers[0].url` del documento OpenAPI debe estar dentro de los `servers` del manifiesto.
- `tools[].name` corresponde al `operationId` de cada operación de OpenAPI; las herramientas de consulta se marcan con `readOnly` y las de escritura no.
- El modo de credenciales (`credential.mode`) solo se necesita al llamar: `shared` / `user_secret` / `ee_jwt` / `oauth`; elige el que el otro sistema pueda aceptar.

## Confirma antes de entregar

| Comprobación | Dónde confirmarla |
| --- | --- |
| Campos del manifiesto y versión del protocolo | `dsh-ee schema get manifest`, `dsh-ee extension check --json` |
| Documentación de la API y mapeo de herramientas | El resultado `tools` de `check`; si las operaciones de escritura están marcadas como `readOnly` |
| Las credenciales solo se obtienen de dsh-ee y no se escriben en el manifiesto | Ni el manifiesto ni el código deben contener claves en texto plano |
| Coherencia del protocolo de la API del servicio | Los casos de `dsh-ee extension test` |
| Empaquetado de la declarativa | `dsh-ee extension pack` genera el zip; la extensión de servicio entrega un repositorio de código o una imagen |
| Quién puede usar qué herramientas | El administrador lo configura en «Conexiones de empresa» y «Campos y reglas de acceso» |

## Relación con las habilidades de DSH

Para dar al agente unas instrucciones ejecutables, dsh-ee ha convertido el flujo de desarrollo de extensiones en una habilidad (`dsh-ee-extension-dev`) que se distribuye a DSH con el permiso «Desarrollar extensiones» (`extensions.develop`). Las personas y el agente ven los mismos pasos: la página es para que la lea una persona y la habilidad es para que la ejecute el agente.

## Texto completo en el repositorio

Esta página del sitio es solo una introducción; los detalles del protocolo, la instalación, la revisión y los códigos de error se rigen por la documentación del repositorio (los enlaces apuntan a la documentación ya fusionada):

- [Desarrollo de extensiones (docs/dev/extensions.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/extensions.md)
- [Resumen del protocolo de extensiones v1 (docs/extension-protocol/README.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [Texto del protocolo de extensiones (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [Publicación empresarial, revisión y permisos (docs/enterprise-publishing.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Directorio de extensiones e índice de ejemplos (extensions/README.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/README.md)
- [JSON Schema del manifiesto (extensions/schemas/manifest.schema.json)](https://github.com/hikariming/dsh-ee/blob/main/extensions/schemas/manifest.schema.json)
- [Decisiones de diseño (docs/plugins.md, borrador de diseño, no es documentación de uso)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugins.md)
