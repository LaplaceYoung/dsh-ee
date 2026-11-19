/** Mis tokens, administración de tokens y autorización de dispositivos. */
import type zhNs from "../zh/tokens";
import type { Messages } from "../types";

export default {
  "tokens": {
    "myTitle": "Mis tokens",
    "myDesc": "Gestiona en un solo lugar los inicios de sesión de DSH, los modelos, el acceso a npm y las claves de API heredadas.",
    "adminDesc": "Consulta, emite y revoca tokens de empleados y bots.",
    "create": "Crear token",
    "issue": "Emitir token",
    "name": "Nombre",
    "owner": "Cuenta",
    "chooseOwner": "Elegir una cuenta",
    "source": "Origen",
    "clientKind": "Tipo de cliente",
    "clientKinds": {
      "dsh": "DSH",
      "third_party": "Harness de terceros"
    },
    "clientKindHints": {
      "dsh": "Solo para el acceso de DSH. DSH tiene su propia tarjeta de aprobación, así que las escrituras se confirman primero dentro de DSH. No lo uses para otros harnesses o scripts.",
      "third_party": "Para Claude Code, Codex y otros harnesses o scripts. Las operaciones de escritura siempre requieren que un empleado las confirme en la interfaz web."
    },
    "scopesTitle": "Ámbitos",
    "usage": "Usado / límite mensual",
    "lastUsed": "Último uso",
    "status": "Estado",
    "list": "Tokens",
    "count": "{{count}}",
    "empty": "Aún no hay tokens.",
    "search": "Buscar nombre, cuenta, prefijo o dispositivo",
    "limit": "Límite mensual de tokens",
    "days": "Vigencia (días)",
    "unlimited": "Ilimitado",
    "noExpiry": "Sin caducidad",
    "issuedTitle": "Token creado",
    "issuedHint": "El token en texto plano solo se muestra una vez. Cópialo y guárdalo de forma segura ahora.",
    "revoke": "Revocar",
    "adminStart": "Iniciar modo de administración",
    "adminEnd": "Finalizar",
    "adminUntil": "Modo de administración hasta {{time}}",
    "adminTitle": "Iniciar modo de administración",
    "adminModeDesc": "Una vez activado, el DSH que usa el token «{{name}}» obtiene herramientas de administración (usuarios, roles, modelos, revisiones, configuración del sistema…), limitadas a tus propios permisos de administración, y el modo finaliza automáticamente. Todo lo que cambia datos te sigue pidiendo confirmación. El agente no puede activarlo por ti.",
    "adminPassword": "Introduce de nuevo tu contraseña de inicio de sesión",
    "adminDuration": "Duración",
    "adminHours": "{{count}} hora(s)",
    "adminConfirm": "Iniciar",
    "adminOn": "El modo de administración está activo hasta {{time}}",
    "adminEnded": "Modo de administración finalizado",
    "revokeConfirm": "¿Revocar «{{name}}»? Dejará de funcionar de inmediato.",
    "revoked": "Revocado {{name}}",
    "bulkMode": "Revocación masiva por",
    "bulkModes": {
      "department": "Departamento",
      "user": "Cuenta",
      "device": "Dispositivo"
    },
    "bulkTargets": {
      "department": "Departamento",
      "user": "Cuenta",
      "device": "Nombre del dispositivo"
    },
    "bulkPlaceholders": {
      "department": "Departamento",
      "user": "Elegir una cuenta",
      "device": "Por ejemplo, MacBook-Pro"
    },
    "bulkActions": {
      "department": "Revocar todos los tokens del departamento",
      "user": "Revocar los tokens de esta cuenta",
      "device": "Revocar los tokens de este dispositivo"
    },
    "bulkPreview": "Se revocarán {{count}} tokens activos",
    "bulkConfirm": "¿Revocar {{count}} tokens activos de «{{target}}»?",
    "bulkDone": "{{count}} tokens revocados",
    "allSources": "Todos los orígenes",
    "allStates": "Todos los estados",
    "departmentFilter": "Filtrar por departamento",
    "sources": {
      "manual": "Manual",
      "device": "Inicio de sesión de DSH",
      "issued": "Emitido por administración",
      "legacy": "Heredado"
    },
    "scopes": {
      "models": "Modelos",
      "registry:read": "Instalar plugins",
      "registry:publish": "Publicar plugins",
      "hub": "Skills y prompts",
      "connections": "Conexiones",
      "agent": "Autoservicio del agente",
      "config": "Configuración de acceso (dsh-ee config)",
      "admin": "Operaciones de administración"
    },
    "states": {
      "active": "Activa",
      "expired": "Caducada",
      "revoked": "Revocada"
    }
  },
  "device": {
    "title": "Autoriza a DSH a usar tu cuenta de la empresa",
    "desc": "Revisa el dispositivo y los ámbitos solicitados antes de autorizar.",
    "authorizeTitle": "Este dispositivo quiere usar tu cuenta",
    "authorizeDesc": "Comprueba el código, el dispositivo y la IP de origen. Autoriza solo dispositivos en los que confíes.",
    "code": "Código de confirmación",
    "expires": "Caduca",
    "name": "Dispositivo",
    "os": "SO",
    "version": "Versión de DSH",
    "ip": "IP de origen",
    "scopes": "Ámbitos solicitados",
    "replaceNote": "El token anterior de este dispositivo se revocará después de la autorización.",
    "approve": "Autorizar",
    "deny": "Denegar",
    "approved": "Autorizado",
    "approvedHint": "Vuelve a tu terminal / DSH para continuar.",
    "denied": "Denegado",
    "deniedHint": "Se ha cancelado este inicio de sesión del dispositivo. Puedes cerrar esta página.",
    "inputCode": "Introduce el código de confirmación",
    "inputHint": "Se ignoran las mayúsculas y los guiones.",
    "continue": "Continuar"
  }
} satisfies Messages<typeof zhNs>;
