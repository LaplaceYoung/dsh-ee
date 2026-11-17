# Permisos y reglas de acceso: referencia rápida

## Tres niveles

El control de acceso de dsh-ee se divide en tres niveles; todos se modifican en la consola de administración y los cambios se aplican de inmediato.

| Nivel | Qué pregunta responde | Dónde se configura | A quién afecta tras configurarlo |
| --- | --- | --- | --- |
| Código de permiso | ¿Puede esta persona usar esta función? | «Roles y permisos» | Visibilidad de páginas y llamadas a la API |
| Campos y reglas de acceso | ¿Se pueden ver o modificar este campo y este dato? | «Campos y reglas de acceso» | Campos extendidos de documentos, bases de conocimiento, usuarios, etc. |
| Restricciones de administración | ¿Requiere aprobación modificar una configuración crítica? | «Campos y reglas de acceso» | Operaciones que cambian valores de campos o configuración |

## Cómo llegan los códigos de permiso a las páginas y a la API

El menú solo decide si algo se ve; el límite real está en la API. Ocultar un botón en la página no constituye una frontera de seguridad.

| Código de permiso habitual | Función | ¿Lo tienen los empleados por defecto? |
| --- | --- | --- |
| `portal.view` | Usar páginas de empleado como el Banco de trabajo, las aplicaciones de empresa y la documentación de desarrollo | Sí |
| `registry.publish` | Publicar plugins, habilidades, prompts y aplicaciones | Sí |
| `extensions.develop` | Obtener la habilidad y las instrucciones de comandos para desarrollar extensiones | No; lo concede un administrador |
| `plugins.manage` | Revisar y publicar o retirar capacidades | No |
| `connections.manage` | Conectar y configurar conexiones de empresa | No |

Los códigos de permiso se sincronizan a la base de datos desde un catálogo integrado en el código; los códigos nuevos de una versión se conceden automáticamente al rol «Empleado» (si el catálogo marca un rol predeterminado).

## Orden de evaluación de las reglas de acceso

1. Superadministrador: no está sujeto a las reglas (salvo que el sistema haya desactivado el superadministrador).
2. Permisos de rol: primero se decide si la función en sí es visible.
3. Reglas de acceso a campos: se decide campo por campo entre read / write / deny; las reglas pueden llevar condiciones.
4. Restricciones de administración: si una operación de escritura debe enviar antes un cambio y quién lo aprueba.

Cuando un nivel anterior llega a una conclusión, los niveles posteriores no pueden ampliarla.

## Dónde configurar y ejemplos

| Quiero | Dónde ir |
| --- | --- |
| Abrir el material de desarrollo de extensiones a cierto tipo de empleado | «Roles y permisos» → conceder `extensions.develop` |
| Que un campo solo lo vea mi departamento | «Campos y reglas de acceso» → crear una regla read y elegir el departamento como condición |
| Prohibir la exportación de un campo «confidencial» de un documento | Igual que arriba: añadir una regla deny y revisar la respuesta de la API |
| Que modificar un campo crítico requiera aprobación | «Campos y reglas de acceso» → restricciones de administración → vincular una solicitud de cambio |

## Cómo colaborar al escribir extensiones

- Declara en el manifiesto los códigos de permiso y nombres de credenciales que necesitas, y no escribas credenciales en el código ni en el manifiesto.
- Las reglas de lectura y escritura de campos las configura el administrador; la extensión solo trabaja con los campos visibles que devuelve la API y no adivina permisos por su cuenta.
- Distingue en los códigos de error «sin permiso», «no existe» y «parámetro incorrecto»; no devuelvas todo como 500.

## Texto completo en el repositorio

Esta página del sitio es solo una referencia rápida; los detalles de las reglas, la aprobación de instalación de extensiones y el catálogo de códigos de permiso están en la documentación del repositorio (los enlaces apuntan a la documentación ya fusionada):

- [Diseño del control de acceso (docs/access-control.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/access-control.md)
- [Publicación empresarial, revisión y permisos (docs/enterprise-publishing.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Resumen del protocolo de extensiones v1 · credenciales y operaciones de escritura (docs/extension-protocol/README.md, disponible tras la fusión)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [Texto del protocolo de extensiones (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [Habilidad de configuración de permisos (extensions/skills/dsh-ee-access-config/SKILL.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/skills/dsh-ee-access-config/SKILL.md)
