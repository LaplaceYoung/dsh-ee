/** «Crear»: mis aplicaciones, copiar desde plantillas, componentes y revisión de «solo yo» y uso compartido. */
import type zhNs from "../zh/build";
import type { Messages } from "../types";

export default {
  "build": {
    "title": "Mis aplicaciones",
    "desc": "Aplicaciones que has creado. Mientras solo las uses tú, basta con guardar para abrirlas en DSH. Para compartirlas con compañeros, amplía su disponibilidad; surte efecto cuando un administrador lo apruebe, y mientras tanto puedes seguir usándolas.",
    "newApp": "Nueva aplicación",
    "fromTemplate": "Empezar desde una plantilla",
    "tabApps": "Aplicaciones {{count}}",
    "tabOthers": "Skills y prompts {{count}}",
    "empty": "Aún no has creado ninguna aplicación. La forma más rápida de empezar es una plantilla: copia una y ajusta la persona y las preguntas iniciales.",
    "emptyOthers": "Aún no has creado skills ni prompts.",
    "stateSelf": "Solo yo",
    "stateShared": "Compartida",
    "stateSharing": "Uso compartido en revisión",
    "statePending": "Pendiente de revisión",
    "stateRejected": "No aprobada",
    "stateUnpublished": "Despublicada",
    "stateNewVersion": "Nueva versión pendiente de revisión",
    "sharingHint": "Solicitud de uso compartido con: {{scope}}. Solo tú puedes usarla hasta que se apruebe.",
    "shareRejected": "Solicitud de uso compartido rechazada: {{note}}",
    "uses": "{{uses}} usos · {{users}} personas en 14 días",
    "edit": "Editar",
    "share": "Compartir…",
    "shareTitle": "Compartir «{{title}}»",
    "shareDesc": "Una aplicación que solo usas tú no necesita revisión. Compartirla con compañeros o departamentos espera la aprobación de un administrador; mientras tanto puedes seguir usándola. Restringir la disponibilidad surte efecto de inmediato.",
    "shareSubmitted": "Solicitud de uso compartido enviada para revisión",
    "shareSaved": "Disponibilidad guardada",
    "useInDsh": "Escribe /{{slug}} en DSH para abrirla"
  },
  "templates": {
    "title": "Empezar desde una plantilla",
    "desc": "Elige la aplicación más parecida, cópiala y hazla tuya. La copia es solo para ti y no necesita revisión.",
    "templates": "Plantillas",
    "templatesEmpty": "Aún no hay plantillas. Los administradores pueden marcar una aplicación como plantilla en el editor de aplicaciones.",
    "company": "Aplicaciones publicadas de la empresa",
    "companyEmpty": "Aún no hay otras aplicaciones publicadas.",
    "copy": "Copiar como mi aplicación",
    "templateBadge": "Plantilla"
  },
  "materials": {
    "title": "Componentes",
    "desc": "Bases de conocimiento, conexiones, skills y prompts con los que crear aplicaciones. Solo se muestran los que puedes usar; las aplicaciones se ejecutan como tú, así que no pueden hacer nada que tú no puedas.",
    "tabs": {
      "knowledge": "Bases de conocimiento",
      "connection": "Conexiones",
      "skill": "Skills",
      "prompt": "Prompts"
    },
    "hints": {
      "knowledge": "Vinculadas a una aplicación, se consultan primero y las respuestas citan sus fuentes.",
      "connection": "Herramientas de sistemas de la empresa. Las herramientas de solo lectura pueden alimentar el banco de trabajo; cada escritura te pide confirmación.",
      "skill": "Formas de trabajo reutilizables de las que las aplicaciones pueden depender.",
      "prompt": "Prompts prácticos que invocas con /nombre en DSH."
    },
    "makeApp": "Crear una aplicación con ello",
    "kbDocs": "{{count}} documentos",
    "kbEmpty": "Aún no hay bases de conocimiento que puedas consultar.",
    "newSkill": "Escribir un skill",
    "newPrompt": "Escribir un prompt",
    "advanced": "Avanzado",
    "advancedHint": "Los plugins necesitan código; la mayoría de las personas no los necesitará.",
    "showAdvanced": "Mostrar opciones avanzadas",
    "hideAdvanced": "Ocultar opciones avanzadas",
    "plugins": "Plugins",
    "sharePlugin": "Subir o solicitar un plugin"
  },
  "selfScope": {
    "self": "Solo yo",
    "selfHint": "Solo tú la usas. Guárdala y ábrela en DSH de inmediato; no necesita revisión.",
    "reviewHint": "Compartirla con otras personas espera la revisión de un administrador; mientras tanto puedes seguir usándola.",
    "copyOf": "{{title}} (copia)",
    "template": "Marcar como plantilla",
    "templateDesc": "Aparece en «Empezar desde una plantilla» de todos"
  },
  "scopeReview": {
    "tab": "Solicitudes de uso compartido",
    "empty": "No hay solicitudes de uso compartido pendientes de revisión.",
    "from": "Ahora",
    "to": "Solicitado",
    "approved": "Uso compartido de «{{title}}» aprobado",
    "rejected": "Uso compartido de «{{title}}» rechazado",
    "hint": "Una vez aprobado, todo el mundo en el nuevo ámbito lo ve en DSH de inmediato. El motivo del rechazo se muestra al propietario."
  }
} satisfies Messages<typeof zhNs>;
