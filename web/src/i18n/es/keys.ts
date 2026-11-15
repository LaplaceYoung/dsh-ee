/** Cuota y claves de API (empleados), y textos comunes sobre claves y cuotas. */
import type zhNs from "../zh/keys";
import type { Messages } from "../types";

export default {
  "keys": {
    "state": {
      "active": "Activa",
      "revoked": "Revocada",
      "expired": "Caducada"
    },
    "quota": {
      "unassigned": "Sin asignar",
      "exhausted": "Agotada",
      "exact": "{{used}} de {{limit}} tokens usados este mes ({{pct}}%)"
    },
    "issued": {
      "title": "Creada «{{name}}»",
      "hint": "La clave completa solo se muestra esta vez y no se podrá volver a ver al cerrar esto. Cópiala ahora en DSH o en la configuración de tu cliente.",
      "close": "Ya la he guardado, cerrar"
    },
    "quotaCard": {
      "title": "Cuota de este mes",
      "desc": "La cuota pertenece a tu cuenta y la comparten todas sus claves. Se restablece el día 1 de cada mes (UTC).",
      "unlimited": "Ilimitada",
      "unassigned": "Sin asignar",
      "unlimitedLabel": "Sin límite",
      "waiting": "Esperando a un administrador",
      "remaining": "Tokens restantes este mes",
      "used": "Usados {{value}}",
      "monthUsed": "Usados este mes",
      "monthlyQuota": "Cuota mensual",
      "allowedModels": "Modelos permitidos",
      "activeKeys": "Claves activas",
      "unassignedNote": "Aún no se te ha asignado ninguna cuota. Puedes crear claves ahora, pero las llamadas devolverán 429 hasta que se defina una cuota. Pide a un administrador que la asigne en Cuotas.",
      "exhaustedNote": "La cuota de este mes está agotada. Vuelve el día 1 (UTC); si necesitas más antes, pide a un administrador.",
      "expiredNote": "Tu cuota caducó el {{time}}, por lo que las llamadas se rechazan. Pide a un administrador que la renueve.",
      "validUntil": "Tu cuota es válida hasta {{time}}."
    },
    "create": {
      "title": "Nueva clave de API",
      "maxed": "Has alcanzado el límite de {{max}} claves activas. Revoca antes una que ya no uses.",
      "name": "Nombre",
      "nameHint": "Algo que te diga para qué sirve, p. ej. «Mi DSH» o «Scripts locales»",
      "namePlaceholder": "Mi DSH",
      "expires": "Fecha de caducidad",
      "expiresHint": "Déjalo vacío para que no caduque",
      "submit": "Crear clave"
    },
    "page": {
      "title": "Cuota y claves de API",
      "desc": "Los administradores asignan a cada cuenta una cuota mensual que comparten todas sus claves. Tú creas las claves y puedes revocarlas en cualquier momento.",
      "newKey": "Nueva clave",
      "myKeys": "Mis claves",
      "activeOf": "{{active}} activas · máx. {{max}}",
      "empty": "Aún no hay claves. Usa «Nueva clave» en la esquina superior derecha. Las llamadas se pausan cuando se agota la cuota.",
      "colName": "Nombre",
      "colUsage": "Uso de esta clave",
      "colLastUsed": "Último uso",
      "colState": "Estado",
      "expiresAt": "Caduca el {{time}}",
      "revoke": "Revocar",
      "revokeConfirm": "Después de revocar «{{name}}», los clientes que la usen dejarán de funcionar de inmediato. ¿Continuar?",
      "revoked": "Revocada «{{name}}»",
      "howTo": "Cómo usarla",
      "howToDesc": "dsh-ee expone una API compatible con OpenAI. Apunta tu cliente a la URL base de abajo y usa tu clave con prefijo sk-ee-; el uso se descuenta de la cuota de tu cuenta.",
      "envVars": "Variables de entorno",
      "yourKey": "tu-clave",
      "hello": "Hola"
    }
  }
} satisfies Messages<typeof zhNs>;
