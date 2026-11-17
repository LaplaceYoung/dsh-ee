# Normas y comprobaciones: ENTRY / FLOW / BOOST

## Qué responde cada una de las tres capas

Las «normas» proceden de los proyectos upstream dsh-plugin-guide / dsh-primer y dsh-standards. dsh-ee **no ejecuta checkers integrados en el servidor**; la documentación del repositorio sí puede citar o adaptar el texto upstream según su propia licencia: CC BY 4.0 exige conservar TASL, el aviso de cambios (Change Notice) y el descargo de responsabilidad en la misma vista; Apache-2.0 exige conservar LICENSE/NOTICE. La guía del sitio solo incluye resúmenes y enlaces; el texto adaptado y la atribución por documento están en el repositorio [`docs/plugin-dev/standards/` (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) y [`docs/dev/standards.md` (disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md).

| Capa | Qué pregunta responde | Quién la usa | Dónde encaja en el sistema |
| --- | --- | --- | --- |
| ENTRY | Si se sostiene la forma mínima de un plugin (punto de entrada, manifiesto, licencia) | El autor y el CI de su repositorio | Se alinea con la comprobación de forma del paquete que hace la verificación previa de publicación |
| FLOW | Si el flujo de uso real del plugin se sostiene (instalación, activación, desinstalación, recuperación de fallos) | El autor y las pruebas | La guía de ejemplos explica cómo reproducirlo |
| BOOST | Si la experiencia y la composibilidad están a la altura (comandos, renderizado, habilidades, múltiples superficies) | El autor y los revisores | Los ejemplos se clasifican y muestran por las superficies implicadas |

## Cómo se usa ENTRY / FLOW / BOOST en el lado de la empresa

| Uso | Cómo se hace | Qué no hacer |
| --- | --- | --- |
| Evidencia del autor | Ejecutar el checker upstream en el CI de su repositorio (`dsh-standards@1.6.9`, requiere Node ≥24.9) y fijar el comando y la versión | No tomar el resultado de la comprobación upstream como conclusión de admisión de la empresa |
| Admisión de la empresa | Tras subir el paquete, mirar el detalle de la verificación previa de dsh-ee (pass / warn / fail y conflictos de publicación) | No sustituir la verificación previa por la comprobación upstream |
| Referencia para la revisión | Durante la revisión, preguntar contrastando ejemplos y cláusulas de las normas | No asignar niveles ni orden a plugins o ejemplos desde la página |
| Ruta de aprendizaje | Leer el código de los ejemplos de lo básico a lo avanzado | No presentar los ejemplos como recomendación oficial |

## Lo que dsh-ee no hace

- No ejecuta checkers de terceros en el servidor ni ofrece una caja de arena donde ejecutar comprobaciones.
- No convierte los resultados de las comprobaciones upstream en niveles de calidad, puntuaciones u orden.
- No presenta el texto adaptado de las normas como si fuera el original upstream: toda cita o adaptación debe conservar la fuente, la licencia (CC BY 4.0 / Apache-2.0) y el Change Notice, con atribución por documento en las adaptaciones.
- No promete «recomendación oficial»: los ejemplos solo indican qué tipo de capacidad muestran.

## Cómo deja constancia el autor

1. Ejecuta en tu máquina la versión fijada `dsh-standards@1.6.9` (Node ≥24.9) y escribe el comando en el CI de tu repositorio.
2. Guarda el informe JSON o HTML como artefacto de compilación y anota la fecha de generación y la versión del host.
3. Cita la ubicación del informe en las notas de publicación, en lugar de escribir solo una conclusión como «ya está comprobado».
4. Cuando se actualicen las normas upstream, vuelve a ejecutarlo; marca la versión en los informes antiguos y no los mezcles.

```bash
# 固定版本，避免 npx 同名包冲突
npx dsh-standards@1.6.9 check .
npx dsh-standards@1.6.9 check . --json > report.json
```

El código de salida del checker, los campos del informe y la forma de leer «not-run / NA / static no equivale a aprobado» se rigen por la referencia para autores del repositorio (ver los enlaces de abajo).

## Relación con las páginas de ejemplos

Las páginas de ejemplos muestran «un recorrido mínimo completo y ejecutable»: cada ejemplo explica qué se puede aprender, los requisitos previos, la versión fijada y el comando de instalación, y en la página de detalle muestra la fuente y la licencia. Los ejemplos no incluyen conclusiones de comprobación y la página tampoco los ordena según los resultados de las comprobaciones.

## Texto completo en el repositorio

Esta página del sitio es solo una introducción; el texto de las normas, el índice de cláusulas y el uso del checker están en la documentación del repositorio y en los repositorios upstream (los enlaces apuntan a la documentación ya fusionada):

- [Normas y comprobaciones (docs/dev/standards.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md)
- [Referencia para autores de normas y checkers (docs/plugin-dev/standards/README.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md)
- [Comandos del checker y campos del informe (docs/plugin-dev/standards/checker-cli.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/checker-cli.md)
- [Límites de la evidencia y lecturas erróneas habituales (docs/plugin-dev/standards/evidence-and-misreadings.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/evidence-and-misreadings.md)
- [Mapeo entre normas y umbrales de admisión (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Upstream: repositorio de dsh-standards](https://github.com/PerryLink/dsh-standards) · [npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)
- [Upstream: dsh-primer (guía de desarrollo de plugins de DSH y cadena de herramientas)](https://github.com/PerryLink/dsh-primer)
