/** Términos comunes, shell de la aplicación y nombres de página predeterminados. */
import type zhNs from "../zh/common";
import type { Messages } from "../types";

export default {
  "common": {
    "revealSecret": "Mostrar en texto plano",
    "hideSecret": "Ocultar texto plano",
    "loading": "Cargando…",
    "loadFailed": "Error al cargar: {{error}}",
    "retry": "Reintentar",
    "save": "Guardar",
    "saving": "Guardando…",
    "cancel": "Cancelar",
    "delete": "Eliminar",
    "edit": "Editar",
    "close": "Contraer",
    "create": "Crear",
    "creating": "Creando…",
    "submitting": "Enviando…",
    "enabled": "Habilitado",
    "copy": "Copiar",
    "copied": "Copiado",
    "copyFailed": "Error al copiar; selecciona el texto manualmente",
    "none": "—",
    "all": "Todos",
    "search": "Buscar",
    "unlimited": "Ilimitado",
    "allModels": "Todos los modelos",
    "never": "Nunca",
    "yes": "Sí",
    "no": "No"
  },
  "shell": {
    "limited": {
      "employee": "Abriste esta página desde DSH y se inició sesión automáticamente. Las funciones de seguridad de la cuenta, como Mis tokens, requieren tu contraseña.",
      "admin": "Abriste esta página desde DSH y se inició sesión automáticamente con acceso solo de empleado. Inicia sesión con tu contraseña para usar las funciones de administración.",
      "action": "Iniciar sesión con contraseña"
    },
    "demo": "Esta es una demo pública compartida por todos. No introduzcas claves ni datos reales.",
    "demoReset": "Esta es una demo pública compartida por todos; los datos se restablecen {{reset}}. No introduzcas claves ni datos reales.",
    "demoPersona": {
      "you": "Demo: ahora eres {{name}} ({{dept}} · {{title}}).",
      "tryInDsh": "La página web muestra el banco de trabajo de la aplicación. Conecta DSH, escribe /{{app}} y pregunta «{{q}}».",
      "switch": "Cambiar de persona",
      "workbench": "Mi banco de trabajo"
    },
    "demoNext": {
      "left": "Próximo restablecimiento en {{left}} ({{time}})",
      "hm": "{{h}} h {{m}} min",
      "m": "{{m}} min",
      "soon": "El restablecimiento es inminente. Lo que estés haciendo puede perderse."
    },
    "language": "Idioma",
    "theme": {
      "label": "Tema: {{mode}}",
      "light": "Claro",
      "dark": "Oscuro",
      "system": "Seguir el sistema"
    },
    "loadingAuth": "Comprobando tu sesión…",
    "loadingMenus": "Cargando el menú…",
    "menusFailed": "No se pudo cargar el menú. Haz clic para reintentar",
    "logout": "Cerrar sesión",
    "openNav": "Abrir navegación",
    "closeNav": "Cerrar navegación",
    "off": "Desactivado",
    "noPages": "Aún no hay páginas disponibles",
    "noPagesHint": "Pide a un administrador que asigne un rol a tu cuenta.",
    "notFound": "Página no encontrada o sin acceso",
    "notFoundHint": "Si la ruta de esta página acaba de cambiar, vuelve a abrirla desde el menú de la izquierda.",
    "noRoles": "Sin roles"
  },
  "table": {
    "search": "Buscar…",
    "range": "{{from}}–{{to}} de {{total}}",
    "perPage": "Por página",
    "prev": "Página anterior",
    "next": "Página siguiente",
    "actions": "Acciones",
    "more": "Más acciones"
  },
  "pickers": {
    "choose": "Seleccionar…",
    "search": "Buscar…",
    "noResults": "Sin coincidencias",
    "none": "Ninguno",
    "pickDate": "Elegir una fecha",
    "clear": "Limpiar"
  },
  "confirm": {
    "delete": {
      "title": "¿Eliminar?",
      "action": "Eliminar"
    },
    "revoke": {
      "title": "¿Revocar?",
      "action": "Revocar"
    },
    "retire": {
      "title": "¿Retirar esta clave?",
      "action": "Retirar"
    },
    "revokeKey": {
      "title": "¿Revocar esta clave?",
      "action": "Revocar"
    },
    "disable": {
      "title": "¿Deshabilitar?",
      "action": "Deshabilitar"
    },
    "reset": {
      "title": "¿Restaurar los valores predeterminados?",
      "action": "Restaurar valores predeterminados"
    },
    "restore": {
      "title": "¿Restaurar?",
      "action": "Restaurar"
    },
    "unbind": {
      "title": "¿Desvincular?",
      "action": "Desvincular"
    },
    "rotate": {
      "title": "¿Rotar la clave?",
      "action": "Rotar"
    }
  },
  "api": {
    "unreachable": "No se puede conectar con el servidor. Comprueba que dsh-ee server esté en ejecución",
    "serverError": "Error del servidor o el servidor no está en ejecución",
    "requestFailed": "Error en la solicitud ({{status}})"
  },
  "pages": {
    "workbench": "Mi banco de trabajo",
    "apps": "Aplicaciones de la empresa",
    "market": "Centro de capacidades",
    "app-new": "Nueva aplicación",
    "build": "Mis aplicaciones",
    "templates": "Empezar desde una plantilla",
    "materials": "Componentes",
    "dsh-download": "Descargar DSH",
    "setup": "Guía de conexión",
    "publish": "Compartir un plugin",
    "dev-docs": "Documentación para desarrolladores",
    "dev-examples": "Ejemplos",
    "my-keys": "Mis tokens",
    "admin-users": "Usuarios",
    "admin-roles": "Roles y permisos",
    "admin-menus": "Menús y páginas",
    "admin-fields": "Campos y reglas de acceso",
    "admin-providers": "Acceso a modelos",
    "admin-quotas": "Cuotas",
    "admin-keys": "Gestión de tokens",
    "admin-dashboard": "Resumen general",
    "admin-stats": "Estadísticas de uso",
    "admin-audit": "Registro de auditoría",
    "admin-plugins": "Capacidades",
    "admin-connections": "Conexiones",
    "admin-review": "Cola de revisión",
    "admin-registry": "Registro npm",
    "admin-upstream": "Precarga de upstream",
    "admin-distribution": "Distribución de DSH",
    "admin-settings": "Configuración",
    "admin-brand": "Marca y apariencia",
    "admin-auth": "Inicio de sesión",
    "admin-signing-keys": "Firma de extensiones",
    "admin-expert-center": "Experts & connectors policy",
    "admin-directory": "Directorio empresarial",
    "knowledge": "Consultas de conocimiento"
  },
  "pageDesc": {
    "workbench": "Estadísticas personales, capacidades instaladas, selecciones del equipo y mis contribuciones",
    "apps": "Aplicaciones de la empresa que puedo usar, abiertas en DSH",
    "market": "Enlace antiguo del hub; redirige a las aplicaciones de la empresa o a los componentes",
    "app-new": "Crea una aplicación y pruébala tú primero",
    "build": "Aplicaciones que hice: personales, compartidas y su estado de revisión",
    "templates": "Copia una plantilla o una aplicación publicada y adáptala",
    "materials": "Bases de conocimiento, conexiones, skills y prompts para crear aplicaciones",
    "dsh-download": "Descarga DSH core y la versión de escritorio",
    "setup": "Conéctate a la fuente de la empresa con un solo comando",
    "publish": "Sube un plugin o solicita uno de la comunidad",
    "dev-docs": "Puntos de entrada para desarrolladores: plugins, extensiones, permisos y publicación",
    "dev-examples": "Ejemplos seleccionados de plugins DSH: versiones fijadas, comandos de instalación, fuentes y licencias",
    "my-keys": "Mi cuota; crea y revoca mis propios tokens",
    "admin-users": "Cuentas, deshabilitación, restablecimiento de contraseñas y roles",
    "admin-roles": "Roles, permisos y concesiones",
    "admin-menus": "Nombres de páginas, rutas, iconos, grupos y acceso",
    "admin-fields": "Campos personalizados, reglas de acceso por campo, restricciones de administración y solicitudes de cambio",
    "admin-providers": "Servicios de modelos upstream, claves de la empresa y modelos públicos",
    "admin-quotas": "Cuotas mensuales de tokens y modelos permitidos por usuario",
    "admin-keys": "Tokens de todos los empleados: consultar, emitir y revocar",
    "admin-dashboard": "Resumen integral: activos, gobernanza, personas y dispositivos",
    "admin-stats": "Instalaciones, uso de modelos, departamentos y almacenamiento",
    "admin-audit": "Inicios de sesión, permisos, configuración, publicación y revisión",
    "admin-plugins": "Publicar y retirar capacidades",
    "admin-connections": "Conecta servidores MCP y API HTTP, gestiona credenciales, configura herramientas y acceso",
    "admin-review": "Cola de revisión",
    "admin-registry": "Reglas de enrutamiento, upstream y tokens de acceso",
    "admin-upstream": "Sincronización en línea e importación sin conexión",
    "admin-distribution": "Distribución de DSH",
    "admin-settings": "Configuración global",
    "admin-brand": "Nombre de la empresa, logo, título de inicio y colores en DSH",
    "admin-auth": "LDAP / AD e inicio de sesión único (OIDC)",
    "admin-signing-keys": "Claves públicas de confianza para la firma de extensiones: añadir, retirar y revocar",
    "admin-expert-center": "Categories, featured items, block policies and per-person access lookup for experts, skills and connectors",
    "admin-directory": "Directorio empresarial registrado: validar/registrar el JSON, instalar y actualizar entradas",
    "knowledge": "Políticas, manuales y documentos; pregunta en Consultas de conocimiento de DSH"
  },
  "menuGroups": {
    "用": "Usar",
    "造": "Crear",
    "接入": "Conectar",
    "开发": "Desarrollar",
    "工作台": "Banco de trabajo",
    "账号与模型": "Cuentas y modelos",
    "管理": "Administración",
    "运营": "Operaciones",
    "账号与权限": "Cuentas y acceso",
    "模型与额度": "Modelos y cuotas",
    "能力与连接": "Capacidades",
    "分发与源": "Distribución",
    "系统": "Sistema"
  },
  "nav": {
    "work": "Banco de trabajo",
    "admin": "Administración",
    "search": "Buscar funciones y ajustes",
    "searchTitle": "Buscar",
    "searchPlaceholder": "Busca páginas, documentación y ajustes, p. ej. «logo», «modelo predeterminado», «código de dispositivo»…",
    "empty": "No se encontraron páginas, documentos ni ajustes",
    "pages": "Páginas",
    "docs": "Documentación",
    "settings": "Ajustes",
    "toc": "En esta página",
    "related": "Páginas relacionadas"
  },
  "searchItems": {
    "settings-basics": "Nombre de la organización, URL de acceso y ámbito de la empresa",
    "settings-scoring": "Puntuación de plugins y puntuación mínima de precarga",
    "settings-admission": "Admisión de plugins (revisión / abierta)",
    "settings-upstream": "Registro npm upstream, modo sin conexión y sincronización de versiones",
    "settings-distribution": "Página de descarga de DSH",
    "settings-connect": "Plugin de conexión de DSH: modelo predeterminado, publicación y Agent",
    "settings-demo": "Importar la empresa de ejemplo «Xinghe Tech»",
    "brand-basics": "Interruptor de marca, nombre de la barra lateral y título de inicio",
    "brand-logo": "Subida de logo y forma del logo",
    "brand-theme": "Colores de DSH: acento, barra lateral y preajustes",
    "brand-preview": "Vista previa de la marca",
    "auth-ldap": "Inicio de sesión y sincronización LDAP / AD",
    "auth-oidc": "Inicio de sesión único (OIDC / SSO)",
    "account": "Mi cuenta: contraseña y perfil"
  },
  "status": {
    "pending": "Pendiente de revisión",
    "published": "Publicado",
    "rejected": "Rechazado",
    "unpublished": "Despublicado"
  },
  "kinds": {
    "skill": "Skill",
    "connector": "Conector",
    "knowledge": "Paquete de conocimiento",
    "plugin": "Plugin"
  },
  "plugin": {
    "noDescription": "Sin descripción",
    "internal": "Interno · {{kind}}",
    "community": "Comunidad · {{category}}",
    "uncategorized": "Sin categoría",
    "featured": "Destacado",
    "needsInternet": "Requiere internet",
    "installs": "{{count}} instalaciones",
    "score": "Puntuación {{score}}"
  }
} satisfies Messages<typeof zhNs>;
