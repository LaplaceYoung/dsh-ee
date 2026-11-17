/** Dependencias y solicitudes de acceso de las aplicaciones (M7). */
import type zhNs from "../zh/requests";
import type { Messages } from "../types";

export default {
  "requests": {
    "kinds": {
      "connection": "Conexión",
      "skill": "Skill",
      "knowledge": "Base de conocimiento",
      "plugin": "Plugin",
      "extension": "Extensión"
    },
    "status": {
      "ok": "Disponible",
      "off": "No habilitado",
      "missing": "Sin acceso",
      "partial": "Parcialmente disponible",
      "gone": "Falta o está despublicado"
    },
    "state": {
      "pending": "Pendiente",
      "approved": "Aprobada",
      "rejected": "Rechazada"
    },
    "depsTitle": "Dependencias y acceso",
    "depsHint": "Las partes a las que no tienes acceso no funcionarán en DSH. Las solicitudes van al propietario o a un administrador; una vez aprobadas, DSH se actualiza solo, sin reiniciar.",
    "partialHint": "{{tools}} de {{total}} herramientas disponibles; el resto requiere roles específicos",
    "pending": "Pendiente",
    "request": "Solicitar",
    "again": "Volver a solicitar",
    "requestAll": "Solicitar todo ({{count}})",
    "rejectedNote": "Rechazada: {{note}}",
    "dialogTitle": "Solicitar acceso a «{{title}}»",
    "dialogDesc": "Explica para qué lo necesitas para que quien apruebe pueda decidir.",
    "forApp": "Para usar la aplicación «{{app}}». Añade una nota si quieres.",
    "forAppShort": "Para ",
    "reason": "Finalidad",
    "reasonOptional": "Nota (opcional)",
    "reasonRequired": "Finalidad (obligatoria)",
    "submit": "Enviar solicitud",
    "submitted": "Solicitado acceso a «{{title}}». Pendiente de aprobación",
    "submittedAll": "Se han enviado {{count}} solicitudes",
    "alreadyPending": "El acceso a «{{title}}» ya está pendiente",
    "cardTitle": "Solicitudes de acceso",
    "toReview": "Por revisar ({{count}})",
    "mine": "Mis solicitudes",
    "approve": "Aprobar",
    "reject": "Rechazar",
    "approved": "Solicitud de {{user}} para «{{title}}» aprobada",
    "rejected": "Solicitud de {{user}} para «{{title}}» rechazada",
    "rejectTitle": "Rechazar la solicitud de «{{title}}»",
    "rejectDesc": "Quien lo solicitó verá tu motivo.",
    "rejectNote": "Motivo",
    "reviewHint": "Aprobar añade a quien lo solicitó al ámbito y le habilita los elementos opcionales.",
    "noteFrom": "Nota de quien aprueba: {{note}}",
    "back": "Volver a mi banco de trabajo",
    "alreadyAllowed": "Ya tienes acceso.",
    "openIt": "Abrirla",
    "pendingSince": "Solicitado el {{time}}, pendiente de aprobación.",
    "lastRejected": "Tu última solicitud fue rechazada."
  }
} satisfies Messages<typeof zhNs>;
