/** Página del registro de auditoría y nombres de acciones. Los nombres se anidan según la acción del servidor (p. ej. auth.login). */
import type zhNs from "../zh/audit";
import type { Messages } from "../types";

export default {
  "auditLog": {
    "title": "Registro de auditoría",
    "desc": "Inicios de sesión, cuentas y permisos, proveedores de modelos, configuración, publicación y revisión. Los registros solo se añaden.",
    "search": "Buscar por actor o destino…",
    "allCategories": "Todas las categorías",
    "categories": {
      "auth": "Inicio de sesión",
      "user": "Usuarios",
      "role": "Roles",
      "permission": "Permisos",
      "menu": "Menús",
      "provider": "Proveedores",
      "model": "Modelos",
      "files": "DeepSeek files",
      "key": "Claves de API",
      "settings": "Configuración",
      "ldap": "LDAP",
      "oidc": "Inicio de sesión único",
      "package": "Capacidades",
      "review": "Revisión",
      "upstream": "Precarga",
      "npm_token": "Tokens npm",
      "token": "Tokens de acceso",
      "device": "Inicio de sesión de dispositivos",
      "asset": "Centro de capacidades",
      "connection": "Conexiones",
      "extension": "Extensiones",
      "connect": "Plugin de conexión de DSH",
      "dsh_release": "Distribución de DSH Desktop",
      "embed": "Incrustación entre sitios",
      "brand": "Marca de la empresa",
      "kb": "Base de conocimiento",
      "app_data": "Datos de aplicaciones",
      "data": "Hub de datos",
      "directory": "Directorio empresarial",
      "client": "Clientes de API"
    },
    "colTime": "Hora",
    "colActor": "Actor",
    "colAction": "Acción",
    "colTarget": "Destino",
    "colDetail": "Detalles",
    "colIp": "IP",
    "colSource": "Origen",
    "allSources": "Todos los orígenes",
    "sources": {
      "web": "Web",
      "token": "Token",
      "agent": "Agente",
      "system": "Sistema",
      "client_credentials": "Credenciales de cliente"
    },
    "system": "Sistema",
    "anonymous": "Anónimo",
    "loadMore": "Cargar más",
    "noMore": "No hay más registros",
    "empty": "No hay registros que coincidan.",
    "actions": {
      "auth": {
        "login": "Inició sesión",
        "login_failed": "Error de inicio de sesión",
        "password_change": "Cambió la contraseña"
      },
      "user": {
        "create": "Creó un usuario",
        "update": "Actualizó un usuario",
        "roles": "Asignó roles",
        "quota": "Cambió la cuota",
        "create_bot": "Creó un bot"
      },
      "role": {
        "create": "Creó un rol",
        "update": "Actualizó un rol",
        "delete": "Eliminó un rol"
      },
      "permission": {
        "create": "Creó un permiso",
        "delete": "Eliminó un permiso"
      },
      "menu": {
        "change": "Cambió el menú"
      },
      "provider": {
        "create": "Añadió un proveedor",
        "update": "Actualizó un proveedor",
        "delete": "Eliminó un proveedor"
      },
      "model": {
        "change": "Cambió los modelos"
      },
      "key": {
        "create": "Creó una clave",
        "revoke": "Revocó una clave"
      },
      "settings": {
        "update": "Cambió la configuración"
      },
      "ldap": {
        "update": "Cambió la configuración LDAP",
        "sync": "Sincronización del directorio"
      },
      "package": {
        "publish": "Publicó",
        "status": "Cambió la disponibilidad",
        "edit": "Editó la ficha",
        "import_request": "Solicitó importación"
      },
      "review": {
        "approve": "Aprobó",
        "reject": "Rechazó"
      },
      "upstream": {
        "sync": "Precargó",
        "import": "Importó un paquete",
        "export": "Exportó un paquete"
      },
      "npm_token": {
        "create": "Creó un token npm",
        "revoke": "Revocó un token npm"
      },
      "token": {
        "create": "Creó un token",
        "issue": "Emitió un token",
        "revoke": "Revocó un token",
        "revoke_bulk": "Revocó tokens en bloque",
        "elevate": "Inició el modo de administración",
        "elevate_end": "Finalizó el modo de administración",
        "elevate_expire": "El modo de administración caducó",
        "refresh": "Renovó un token",
        "refresh_reuse": "Detectó reutilización de un token de actualización (familia revocada)"
      },
      "device": {
        "approve": "Autorizó un dispositivo",
        "deny": "Denegó un dispositivo"
      },
      "asset": {
        "create": "Creó una capacidad",
        "version": "Envió una versión",
        "edit": "Cambió la disponibilidad",
        "delete": "Eliminó una capacidad",
        "scope_request": "Solicitó mayor disponibilidad"
      },
      "hub": {
        "category": "Changed a capability category",
        "block": "Changed a block policy"
      },
      "connection": {
        "tool": "Configuró una herramienta de conexión",
        "request": "Solicitó una conexión",
        "credential_set": "Guardó una credencial de conexión",
        "credential_delete": "Eliminó una credencial de conexión",
        "credential_revoke": "Revocó la credencial de conexión de un empleado",
        "ee_jwt_rotate": "Rotó la clave de firma ee_jwt",
        "shared_credential_set": "Guardó una credencial compartida con nombre",
        "shared_credential_delete": "Eliminó una credencial compartida con nombre"
      },
      "extension": {
        "install": "Instaló o actualizó una extensión",
        "rollback": "Revirtió la versión de una extensión"
      },
      "connect": {
        "publish": "Publicó el plugin de conexión"
      },
      "oidc": {
        "update": "Cambió la configuración OIDC",
        "bind": "Vinculó una identidad SSO",
        "feishu_sync": "Sincronizó la organización de Feishu"
      },
      "access": {
        "request": "Solicitó acceso"
      },
      "app": {
        "export": "Exportó una aplicación",
        "import": "Importó una aplicación"
      },
      "demo": {
        "clear": "Borró los datos de la demo",
        "import": "Importó los datos de la demo"
      },
      "feedback": {
        "vote": "Valoró una respuesta"
      },
      "dsh_release": {
        "check": "Comprobó las versiones oficiales",
        "mirror": "Descargó el instalador",
        "mirror_failed": "Error de verificación del instalador",
        "recommend": "Estableció la versión recomendada",
        "delete": "Eliminó el instalador local"
      },
      "brand": {
        "update": "Actualizó la marca de la empresa"
      },
      "data": {
        "upload": "Subió una tabla de datos",
        "drop": "Eliminó una tabla de datos",
        "dataset_create": "Creó un conjunto de datos",
        "dataset_update": "Editó un conjunto de datos",
        "dataset_delete": "Eliminó un conjunto de datos"
      },
      "app_data": {
        "insert": "Añadió datos de aplicación",
        "update": "Editó datos de aplicación",
        "delete": "Eliminó datos de aplicación",
        "export": "Exportó datos de aplicación"
      },
      "kb": {
        "create": "Creó una base de conocimiento",
        "edit": "Editó una base de conocimiento",
        "delete": "Eliminó una base de conocimiento",
        "doc_create": "Añadió un documento",
        "doc_edit": "Editó un documento",
        "doc_delete": "Eliminó un documento"
      },
      "embed": {
        "session_create": "Emitió un código de incrustación",
        "revoke": "Revocó una sesión de incrustación",
        "deny": "Denegó una solicitud de incrustación"
      },
      "directory": {
        "apply": "Registró el directorio empresarial"
      },
      "client": {
        "create": "Creó un cliente de API",
        "revoke": "Deshabilitó un cliente de API"
      },
      "files": {
        "upload": "Uploaded file",
        "delete": "Deleted file",
        "list": "Listed files"
      }
    }
  }
} satisfies Messages<typeof zhNs>;
