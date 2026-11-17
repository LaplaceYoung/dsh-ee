# Entradas de publicación y cómo leer la verificación previa

## Tres entradas de publicación

| Entrada | Ruta | Escenario adecuado | Cuándo se ejecuta la verificación previa |
| --- | --- | --- | --- |
| Subida web | «Compartir plugin» del Banco de trabajo | Primera publicación, para ver la verificación previa mientras corriges | Justo después de subir |
| Publicación directa con npm | `npm publish --registry <registro de la empresa>` | Ya tienes CI y publicas por canalización de versión | Al publicar |
| API de agente | `POST /api/agent/packages` | Publicación automatizada y migraciones masivas | Al publicar |

Las tres entradas usan el mismo conjunto de reglas de admisión: el paquete pasa primero por una verificación estática, después se comprueban los conflictos de publicación y, por último, el modo de admisión decide si se publica directamente o pasa a revisión.

## Qué revisa la verificación previa

La verificación previa solo descomprime el paquete y hace comprobaciones estáticas: **no ejecuta el código del plugin**, no escribe en la base de datos ni ocupa números de versión. El resultado tiene tres niveles: pass / warn / fail.

| Clave de escaneo | Qué le importa | Gravedad |
| --- | --- | --- |
| `bundle` | `dsh.bundle` y el archivo patch: ausencia, rutas inseguras, archivos que no están en el paquete | fail si el paquete es un plugin; aviso si un paquete que no es plugin no lo trae |
| `patch` | Estructura de `cordis.patch.yml` (array, `insert`, `id`/`name`, ids duplicados) | un error de estructura es fail; nombres incoherentes, etc. son aviso |
| `cordis` | Si `dependencies` incluye paquetes relacionados con Cordis | Aviso |
| `cordis-peer` | Si el rango de Cordis en `peerDependencies` se desvía del valor recomendado | Aviso |
| `skills` | Inventario de habilidades distribuidas con el paquete | Aviso (indica que la revisión lo verá) |
| `install-scripts` | Scripts de instalación (`preinstall`, etc.) | Aviso |
| `secrets` | Si el paquete contiene posibles secretos | **fail** |
| `license` | Si se declara una licencia SPDX | **Aviso** (no bloquea) |
| `engines` | Si el plugin declara `engines.dsh` | Aviso |
| `dependencies` | Inventario de dependencias fuera del ámbito de la empresa | Información |
| `size` | Tamaño del paquete (aviso si supera 20 MB) | Aviso |

La verificación previa solo responde a «¿puede la empresa aceptar este paquete de forma segura?»; no evalúa calidad ni asigna nivel u orden a los plugins. Las normas y los checkers pertenecen a la cadena de herramientas upstream y su conclusión es material de autoevidencia del autor, no se integra en la verificación previa de la empresa.

## Cómo leer los resultados

La verificación previa devuelve tres campos de conclusión que se corresponden con el comportamiento real de la API de publicación:

| Campo de conclusión | Significado |
| --- | --- |
| `can_submit` | No hay conflictos de publicación; la API de publicación aceptará el paquete |
| `will_publish` | En el modo abierto, las comprobaciones no tienen elementos fail; al publicar queda disponible de inmediato |
| `requires_review` | Se puede enviar, pero entrará en la cola de revisión |

Lo que realmente decide «publicar directamente o pasar a revisión» es el modo de admisión y los elementos fail:

| Situación | Modo de lista de permitidos (predeterminado) | Modo abierto |
| --- | --- | --- |
| Conflicto de publicación (nombre de paquete / versión ocupada / ámbito / propiedad / paquete upstream con el mismo nombre) | La API de publicación lo rechaza | Igual |
| Tiene elementos fail (como secrets o errores de estructura del bundle) | Se puede enviar y pasa a revisión; **no es un rechazo** | Se puede enviar y pasa a revisión |
| Solo avisos, o todo pass | Pasa a revisión | Sin elementos fail, queda disponible al publicar |

El número de versión queda ocupado en el momento de enviar; las versiones rechazadas o retiradas tampoco se pueden volver a publicar, así que sube el número de versión después de corregir.

## Autocomprobación antes de enviar

1. Empaqueta después de `pnpm build` y confirma que la lista `files` incluye todos los artefactos.
2. Ejecuta en local `dsh-primer check` y `npx dsh-standards@1.6.9 check .`, y deja los resultados en el CI de tu repositorio.
3. No incluyas en el paquete `.tgz`, mapas de código fuente, claves reales en los fixtures de prueba ni rutas absolutas de tu máquina.
4. Indica con claridad `license`, `engines.dsh` y la línea de versión de DSH para no descubrir el desajuste al instalar.
5. Si necesitas scripts de instalación, explica en la nota de envío qué hacen.

## Preguntas frecuentes

| Síntoma | Causa habitual | Solución |
| --- | --- | --- |
| La verificación previa muestra un conflicto de publicación | El nombre del paquete no está en el ámbito de la empresa, la versión ya existe o hay un paquete upstream con el mismo nombre | Estos conflictos hacen que la API de publicación rechace directamente; cambia el nombre del paquete o sube la versión |
| La verificación previa tiene elementos fail | Errores de estructura del bundle/patch o detección de posibles secretos | Corrige antes de enviar; también puedes enviar con elementos fail, pero pasará a revisión y el número de versión quedará ocupado |
| Tras enviar pasa a pendiente de revisión | Modo de lista de permitidos, o modo abierto con elementos fail / avisos | Corrige según los comentarios de la revisión; no pruebes el mismo problema una y otra vez con números de versión nuevos |
| La revisión no llega a una conclusión | Los scripts de instalación, las dependencias o el contenido sensible necesitan confirmación manual | Amplía la explicación en la descripción o contacta con un administrador |
| La versión instalada en la empresa no es la correcta | Caché del cliente o una versión antigua indicada explícitamente | Comprueba la versión en `dsh plugin add <paquete>@<versión>` |

## Texto completo en el repositorio

Esta página del sitio es solo una introducción; el texto completo sobre publicación, revisión y permisos está en la documentación del repositorio (los enlaces apuntan a la documentación ya fusionada):

- [Publicación de plugins de DSH (docs/dev/plugin-publish.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Publicación empresarial, revisión y permisos (docs/enterprise-publishing.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Guía para autores de plugins (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [Mapeo entre normas y umbrales de admisión (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Errores y FAQ (docs/dev/errors-faq.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
