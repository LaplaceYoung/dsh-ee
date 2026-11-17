# Escribe un plugin de DSH y publícalo en el registro de la empresa

## Requisitos previos

| Dependencia | Versión | Por qué hace falta |
| --- | --- | --- |
| Node | ≥ 24.9.0 | Los `engines` de `dsh-primer` y `dsh-standards` exigen ≥24.9; la línea DSH 0.2.2 también exige Node ≥24.9, y las líneas 0.2.x anteriores pueden funcionar en 22.19+ |
| DSH | línea 0.2.x (0.2.1–0.2.2) | Ejecutar el plugin y `dsh plugin add`; `dsh-primer` declara compatibilidad con `>=0.2.1-0 <0.3.0`; rellena `engines.dsh` con el rango realmente verificado |
| pnpm | cualquier versión reciente | Instalar dependencias y empaquetar |
| dsh-primer | 0.4.x | Esqueleto, comprobación estática y prueba de humo de instalación (nombre antiguo `dsh-plugin-guide` 0.3.x, alias antiguo de la CLI `dsh-plugin-dev`) |
| Cuenta de empresa | Con «Publicar capacidades» (`registry.publish`) | Subir plugins y solicitar la incorporación de plugins de la comunidad |

## Crea un esqueleto en cinco minutos

```bash
# 0. dsh-primer 是可安装的 DSH bundle，不是 DSH 自带命令；先装进你用的 profile
dsh plugin --profile web add dsh-primer

# 1. 生成骨架（上游模板，TypeScript）
dsh-primer new my-plugin

# 2. 本地装依赖并跑通
cd my-plugin
pnpm install
pnpm build

# 3. 自查包形状（静态检查，在本地执行）
dsh-primer check

# 4. 装进一个测试 profile 验证加载/卸载
dsh-primer verify
```

`dsh-primer` forma parte de la cadena de herramientas upstream; dsh-ee solo fija la versión y explica su uso, sin replicar su lógica de comprobación.

## Campos del manifiesto que más importan a la empresa

| Campo | Qué escribir | Por qué lo mira la empresa |
| --- | --- | --- |
| `name` | Nombre del paquete bajo el ámbito de la empresa, por ejemplo `@company/hello` | Determina si se puede enrutar al registro npm de la empresa |
| `version` | Versión semántica; una versión por publicación | Una vez ocupado un número de versión no se puede reutilizar; los conflictos de publicación rechazan el envío |
| `main` / `files` | Entrada real que se va a publicar y lista de archivos permitidos | Evita publicar código fuente, pruebas y `.tgz` junto con el paquete |
| `cordis.patch.yml` | Declaración del patch del plugin sobre Cordis | La verificación previa revisa la estructura del bundle y del patch |
| `dsh.bundle` | Manifiesto del plugin de DSH (si lo hay) | Determina si el cliente puede reconocer este paquete |
| `engines.dsh` | Rango de versiones de DSH realmente verificado | Se contrasta con la línea de versión de DSH de la empresa; si no se declara, se emite un aviso |
| `peerDependencies` | Peers relacionados con DSH | La verificación previa revisa si el rango de peers se puede resolver |
| `license` | Identificador SPDX como `Apache-2.0` | La falta de license es un **aviso** de la verificación previa (no bloquea), pero pasa a revisión manual |

## Qué revisan las comprobaciones locales

| Comando | Qué revisa | Dónde se ejecuta |
| --- | --- | --- |
| `dsh-primer check` | Que la forma del paquete y sus puntos de entrada estén completos | En la máquina o el CI del autor |
| `dsh-primer verify` | Prueba de humo de instalación, activación y desinstalación | En la máquina o el CI del autor |
| `npx dsh-standards@1.6.9 check .` | Evidencia estática de las cláusulas de la norma upstream (requiere Node ≥24.9) | En la máquina o el CI del autor; la conclusión es material de autoevidencia del autor |

El servidor de dsh-ee no ejecuta el código del plugin ni ejecuta estos checkers; la verificación previa de «Compartir plugin» en la web solo mira la forma estática del paquete y las condiciones de admisión de la empresa.

## Publica en el registro de la empresa

| Entrada | Cómo | Para quién |
| --- | --- | --- |
| «Compartir plugin» del Banco de trabajo | Sube el `.tgz`, revisa primero el resultado de la verificación previa y luego envía | Primera publicación, para ver antes la verificación previa |
| Publicación directa con npm | `npm publish --registry <registro de la empresa>`, con un token creado en «Mis tokens» | Quien ya tiene una canalización de CI |
| API de agente | `POST /api/agent/packages` con un token de empresa | Publicación automatizada |

El número de versión queda ocupado durante la revisión y la publicación en el catálogo; las versiones repetidas o en conflicto se rechazan. La verificación previa da tres conclusiones: `can_submit` (no hay conflictos de publicación y la API de publicación aceptará el paquete), `will_publish` (en el modo abierto, las comprobaciones no tienen elementos fail y publicar lo deja disponible de inmediato) y `requires_review` (tras el envío pasa a revisión).

## Preguntas frecuentes

| Síntoma | Causa habitual | Solución |
| --- | --- | --- |
| La verificación previa avisa de que falta el archivo de entrada | La lista `files` no incluye los artefactos de compilación, o `main` apunta a un archivo sin compilar | Ejecuta primero `pnpm build` y revisa después `files` |
| La verificación previa tiene elementos fail | Errores de estructura del bundle/patch, detección de posibles secretos, etc. | Corrige antes de enviar; también puedes enviar con elementos fail, pero el número de versión quedará ocupado y la revisión normalmente no pasará |
| La verificación previa solo tiene avisos | Falta license, no se declara `engines.dsh`, hay scripts de instalación, etc. | No bloquea: en el modo de lista de permitidos pasa a revisión manual; en el modo abierto puede publicarse directamente |
| El rango de peers no se puede resolver | Se declaran dos líneas de versión de DSH incompatibles | Deja solo una línea de versión o completa un rango que se pueda resolver |
| Al instalar da error de versión de Node | `engines.node` no coincide con el Node del ordenador del empleado | Corrige `engines.node` según los resultados verificados (ejemplo: `^22.19.0 \|\| >=24.9.0`) |
| No se puede instalar en la empresa | Permisos de token insuficientes o npmrc sin el registro de la empresa | Revisa el permiso «Instalar capacidades» y el `.npmrc` |

## Texto completo en el repositorio

Esta página del sitio es solo una introducción; el texto completo y la licencia están en la documentación del repositorio (los enlaces apuntan a la documentación ya fusionada):

- [Inicio rápido (docs/dev/quickstart.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/quickstart.md)
- [Publicación de plugins de DSH (docs/dev/plugin-publish.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Errores y FAQ (docs/dev/errors-faq.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
- [Guía para autores de plugins (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [Guía de desarrollo de plugins de DSH (docs/plugin-dev/README.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md)
- [Upstream: repositorio de dsh-primer](https://github.com/PerryLink/dsh-primer) · [página del paquete npm](https://www.npmjs.com/package/dsh-primer)
- [Upstream: dsh-plugin-guide (página que apunta al nombre antiguo 0.3.x)](https://github.com/PerryLink/dsh-plugin-guide)
