# Por dónde empezar: tres rutas de desarrollo

## Tres rutas

El «desarrollo» en dsh-ee se divide en tres rutas según lo que entregas: escribir un plugin de DSH, escribir una extensión de dsh-ee o conectar un cliente que no es DSH. Las tres comparten la misma identidad de empresa y el mismo token, pero difieren en código, comprobaciones y entradas de publicación.

| Ruta | Qué entregas | Dónde encaja en dsh-ee | Herramienta de inicio |
| --- | --- | --- | --- |
| Autor de plugins de DSH | Un paquete de plugin de DSH (herramientas, cliente, servicios, comandos, etc.) | Registro npm de la empresa, verificación previa de publicación, revisión y distribución | Primero ejecuta `dsh plugin add dsh-primer`; después crea el esqueleto con `dsh-primer new` y compruébalo con `dsh-primer check` |
| Autor de extensiones de dsh-ee | Una extensión: declarativa / de servicio / local (la integrada la implementa dsh-ee) | Conexiones de empresa, campos y reglas de acceso, habilidades del agente | Crea el esqueleto con `dsh-ee extension init` y compruébalo con `dsh-ee extension check` |
| Cliente que no es DSH | Un cliente que llama a modelos y a MCP por HTTP | Mis tokens, puerta de enlace de modelos, `/v1`, `/mcp` y el endpoint de descubrimiento | Primero crea un token en «Mis tokens» del Banco de trabajo; la ruta se explica en [Conecta un Harness de terceros o tu propio cliente](/docs/harness-quickstart) |

Una extensión tiene cuatro formas (declarativa / de servicio / local / integrada); en el manifiesto solo puedes declarar las tres primeras como `kind`. Para más detalle, consulta [Escribe una extensión de dsh-ee: manifiesto, comprobaciones y llamadas](/docs/extension-quickstart).

## Primero, distingue «plugin» de «extensión»

Un **plugin de DSH** se instala en el cliente DSH y lo compila y valida el autor en su propia máquina; dsh-ee se encarga de la admisión, el alojamiento y la distribución del lado de la empresa.

Una **extensión de dsh-ee** es una unidad de integración que se conecta al servidor de dsh-ee; la gestiona la propia CLI de dsh-ee y el administrador la configura en «Conexiones de empresa» y «Campos y reglas de acceso».

| Criterio | Plugin de DSH | Extensión de dsh-ee |
| --- | --- | --- |
| Dónde se ejecuta | En la máquina donde está el cliente DSH | En el proceso de dsh-ee, que envía la solicitud (declarativa); en el servicio propio de la empresa (de servicio); en el ordenador del empleado (local) |
| Archivo de manifiesto | `package.json` y `cordis.patch.yml` | `dsh-ee.extension.json` |
| Entrada de publicación | Registro npm de la empresa o «Compartir plugin» del Banco de trabajo | La declarativa se sube como zip; en la de servicio, el administrador indica la dirección del servicio; la local se entrega como paquete npm y manifiesto |
| Quién puede instalarla | Empleados con el permiso «Instalar capacidades» | La configura el administrador en las conexiones y reglas |

## Orden de lectura recomendado

1. Primera publicación de un plugin: [Escribe un plugin de DSH y publícalo en el registro de la empresa](/docs/plugin-quickstart)
2. Primera extensión: [Escribe una extensión de dsh-ee: manifiesto, comprobaciones y llamadas](/docs/extension-quickstart)
3. Para conectar un Harness de terceros o un cliente propio: [Conecta un Harness de terceros o tu propio cliente](/docs/harness-quickstart)
4. Para confirmar los límites de permisos: [Permisos y reglas de acceso: referencia rápida](/docs/access-rules)
5. Si antes de enviar quieres saber qué te van a preguntar: [Entradas de publicación y cómo leer la verificación previa](/docs/publish-precheck)
6. Para entender la frontera entre normas y comprobaciones: [Normas y comprobaciones: ENTRY / FLOW / BOOST](/docs/quality-standards)

## Qué más hay que preparar

| Elemento | Descripción |
| --- | --- |
| Cuenta y permisos | Para publicar plugins se necesita el permiso «Publicar capacidades» (`registry.publish`); para el material de desarrollo de extensiones, el permiso «Desarrollar extensiones» (`extensions.develop`) |
| Dirección del registro de la empresa | En la «Guía de conexión» se obtienen la dirección del registro npm, la configuración del token y el uso de `setup.sh` |
| Cadena de herramientas local | Node ≥24.9 (lo exigen los `engines` de `dsh-primer` y `dsh-standards`) y pnpm; solo para escribir extensiones de servicio se necesita Go 1.27+ |
| Red | En un despliegue en la intranet, GitHub y el npm público pueden no ser accesibles; confirma primero que puedes obtener las dependencias y, si hace falta, usa la «precarga upstream» |

## Guía del sitio y texto completo del repositorio

Las páginas del sitio solo recogen resúmenes y puntos de entrada desde la perspectiva de dsh-ee y no copian el texto upstream; la documentación del repositorio sí puede citar o adaptar material upstream según su propia licencia (CC BY 4.0 exige conservar TASL, el aviso de cambios —Change Notice— y el descargo de responsabilidad en la misma vista; Apache-2.0 exige conservar LICENSE/NOTICE). El texto, las licencias y las versiones upstream se rigen por la documentación del repositorio y los repositorios upstream (los enlaces apuntan a la documentación ya fusionada):

| Contenido | Dónde |
| --- | --- |
| Índice de documentación de desarrollo (inicio rápido / extensiones / Harness / publicación / normas / errores y FAQ) | [docs/dev/README.md (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/README.md) |
| Guía de desarrollo de plugins de DSH (capa de referencia local de la guía comunitaria) | [docs/plugin-dev/README.md (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md) |
| Referencia para autores de normas y checkers (ENTRY / FLOW / BOOST, DS-C, checker) | [docs/plugin-dev/standards/README.md (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) |
| Resumen del protocolo de extensiones v1 | [docs/extension-protocol/README.md (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md) |
| Publicación empresarial, revisión y permisos | [docs/enterprise-publishing.md (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md) |

Versiones fijadas de las herramientas upstream: `dsh-primer` ([página del paquete npm](https://www.npmjs.com/package/dsh-primer)) y `dsh-standards@1.6.9` ([npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)); dsh-ee solo fija la versión y explica su uso, sin replicar la lógica de comprobación upstream.
