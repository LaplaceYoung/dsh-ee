/** Pestaña «Que la IA lo haga» de la nueva aplicación y «Panorama de la aplicación» en el detalle. */
import type zhNs from "../zh/app-guide";
import type { Messages } from "../types";

export default {
  "appAgent": {
    "tabAgent": "Que la IA lo cree (recomendado)",
    "tabManual": "Configurar manualmente",
    "intro": "No hace falta rellenar todos los campos aquí: describe lo que quieres, copia el prompt de abajo y envíalo a la IA en DSH. Lee la guía de diseño, comprueba qué sistemas puedes usar, te muestra un plan para confirmar y luego crea una aplicación que solo puedes usar tú.",
    "describeTitle": "1. Describe la aplicación que quieres",
    "describeLabel": "Qué debe hacer",
    "describePlaceholder": "p. ej. Inscripciones para la salida de equipo de la próxima semana: recoger nombre, turno y necesidades dietéticas, contar personas por turno; yo veo las inscripciones de todos",
    "examples": "Prueba una:",
    "example": {
      "signup": {
        "label": "Inscripción a la salida de equipo",
        "text": "Inscripciones para la salida de equipo de la próxima semana: recoger nombre, turno y necesidades dietéticas, contar personas por turno; cada persona solo ve su propia inscripción, yo lo veo todo."
      },
      "visit": {
        "label": "Resumen de visita a cliente",
        "text": "Antes de visitar a un cliente, genera un resumen con un clic: información básica del cliente, pedidos recientes y tickets abiertos, además de tres puntos de conversación."
      },
      "expense": {
        "label": "Asistente de gastos",
        "text": "Responde preguntas sobre la política de gastos, comprueba si mi solicitud de gastos cumple la normativa y enumera los documentos que faltan."
      },
      "ledger": {
        "label": "Registro de préstamo de equipos",
        "text": "Registro de préstamo de equipos del equipo: anota prestatario, dispositivo, fechas de préstamo y devolución, enumera los dispositivos aún no devueltos y destaca los vencidos."
      }
    },
    "usersLabel": "Quién la usa y cómo (opcional)",
    "usersPlaceholder": "p. ej. El equipo de ventas la consulta cada lunes; debe poder rellenarse en el móvil",
    "connectionsLabel": "Sistemas de negocio que usar (opcional)",
    "connectionsHint": "Solo se muestran las conexiones que puedes usar. ¿No estás seguro? Déjalo vacío y la IA lo buscará.",
    "noConnections": "Aún no hay conexiones a sistemas de negocio que puedas usar.",
    "knowledgeLabel": "Bases de conocimiento a consultar (opcional)",
    "noKnowledge": "Aún no hay bases de conocimiento que puedas leer.",
    "needTables": "Almacenar sus propios datos",
    "needTablesDesc": "Inscripciones, registros, libros: datos que produce la propia aplicación. La IA diseña tablas integradas y formularios de entrada.",
    "needWeb": "Abrir también en el navegador y en el móvil",
    "needWebDesc": "Activa la versión web (/a/slug); los formularios se pueden enviar desde el navegador.",
    "promptTitle": "2. Copia el prompt y envíalo a DSH",
    "promptLabel": "Prompt para DSH",
    "copy": "Copiar prompt",
    "copied": "Copiado; pégalo en una conversación nueva de DSH",
    "openDsh": "Abrir DSH",
    "emptyDesc": "Escribe primero una línea sobre lo que quieres.",
    "stepsTitle": "Qué pasa después",
    "steps": [
      "La IA lee el skill dsh-ee-app-design (guía de diseño de aplicaciones) y usa ee_hub para comprobar los sistemas y las API que puedes usar.",
      "Enumera el slug, el título, la persona, las preguntas iniciales, el banco de trabajo y las tablas para que los confirmes; basta con decir qué cambiar.",
      "Cuando confirmes, crea una aplicación solo para mí con ee_hub create (for_me): sin revisión, aparece de inmediato en tu DSH.",
      "Encuéntrala en «Mis aplicaciones»: consulta el panorama, pruébala y cambia el ámbito para compartirla."
    ],
    "notReady": "¿Aún no has instalado o conectado DSH?",
    "goSetup": "Ir a Empezar",
    "iterateTitle": "Sigue mejorándola",
    "iterateDesc": "Dilo en DSH; la IA envía una versión nueva con ee_hub new_version:",
    "iterate": [
      "Añade un gráfico de barras mensual al banco de trabajo de /<slug>",
      "Añade un campo «trae familia» a la tabla de inscripción de /<slug>",
      "Reescribe las preguntas iniciales de /<slug> para el equipo de ventas"
    ],
    "tipsTitle": "Consejos",
    "tips": [
      "¿Ya tienes una gran conversación? Escribe /ee-save en DSH para convertirla en un borrador de aplicación.",
      "¿Quieres ver primero lo que es posible? Escribe /dsh-ee-app-design en DSH y pregunta a la IA por los diseños.",
      "Las aplicaciones nuevas son solo para ti; para compartirlas, cambia el ámbito en la página de la aplicación (puede requerir revisión)."
    ],
    "manualHint": "Para rellenar cada campo tú mismo o ajustar el JSON, usa «Configurar manualmente».",
    "goBuild": "Ver «Mis aplicaciones»",
    "prompt": {
      "head": "Crea una aplicación empresarial en dsh-ee, por ahora solo para mí.",
      "what": "[Qué debe hacer] ",
      "users": "[Quién la usa y cómo] ",
      "connections": "[Sistemas de negocio] {{list}}. Primero confirma que puedo usarlos con ee_hub search (kind: connection) y luego comprueba sus API con detail.",
      "connectionsAuto": "[Sistemas de negocio] Usa ee_hub search (kind: connection) para ver cuáles puedo usar y elige los adecuados; avísame si ninguno encaja.",
      "knowledge": "[Bases de conocimiento] {{list}}",
      "tables": "[Datos] Esta aplicación almacena sus propios datos: diseña tablas integradas (elige visibilidad own / shared / author), con formularios de entrada y listas en el banco de trabajo.",
      "web": "[Versión web] Establece web: true para que los compañeros puedan usarla en el navegador y en el móvil.",
      "steps": "Pasos:\n1. Lee primero el skill dsh-ee-app-design y sigue su diseño de panel y sus reglas para el banco de trabajo{{tables}}.\n2. Muéstrame el slug, el título, la descripción, la persona, 3 preguntas iniciales, los widgets del banco de trabajo y las fuentes de datos para confirmar; no envíes nada antes de que confirme.\n3. Después de que confirme, créala con ee_hub create (kind: app, for_me: true).\n4. Luego dame el enlace web a su página: {{origin}}/assets/<id>",
      "tablesSuffix": " y las tablas"
    }
  },
  "appMap": {
    "title": "Panorama de la aplicación",
    "desc": "De dónde vienen los datos de esta aplicación, qué sistemas y API usa, qué datos puede leer y escribir y qué te aporta.",
    "summary": {
      "systems": "{{count}} sistemas",
      "tools": "{{count}} API",
      "kb": "{{count}} bases de conocimiento",
      "tables": "{{count}} tablas integradas",
      "widgets": "{{count}} widgets del banco de trabajo"
    },
    "sources": "Fuentes de datos",
    "app": "Aplicación",
    "provides": "Qué ofrece",
    "system": "Sistema de negocio",
    "tool": "API",
    "knowledge": "Base de conocimiento",
    "dataset": "Hub de datos",
    "ownTable": "Tabla integrada",
    "skill": "Skill",
    "plugin": "Plugin",
    "fields": "{{count}} campos",
    "read": "Leer",
    "write": "Escribir",
    "workbenchUses": "Usada por el banco de trabajo",
    "toolsHidden": "Aún no puedes ver las API de este sistema",
    "noTools": "No hay API que puedas usar en este sistema",
    "loading": "Cargando API…",
    "noSources": "Sin sistemas ni bases de conocimiento; solo responde con su persona y el modelo.",
    "persona": "Persona",
    "noPersona": "Sin persona definida",
    "chat": "Chat",
    "chatDesc": "Escribe /{{slug}} en DSH",
    "starters": "{{count}} preguntas iniciales",
    "web": "Versión web",
    "webDesc": "Abre /a/{{slug}} en un navegador o en el móvil",
    "workbench": "Banco de trabajo",
    "pane": "Panel «{{title}}»",
    "noProvides": "Aún no hay banco de trabajo; solo se puede usar en el chat.",
    "from": "de {{source}}",
    "writesTo": "envía a «{{table}}»",
    "status": {
      "ok": "Disponible",
      "off": "No habilitado",
      "missing": "Sin acceso",
      "partial": "Parcialmente disponible",
      "gone": "Ya no existe"
    },
    "opsTitle": "Con qué datos puede trabajar",
    "opsDesc": "Según tus permisos actuales. Cada escritura en un sistema de negocio requiere tu confirmación en una tarjeta de aprobación; las tablas integradas las edita la aplicación directamente.",
    "col": {
      "target": "Datos",
      "where": "Desde",
      "can": "Puede",
      "how": "Cómo"
    },
    "can": {
      "query": "Consultar",
      "write": "Escribir / actualizar",
      "kbRead": "Buscar y citar fuentes",
      "datasetRead": "Consulta de solo lectura",
      "tableRead": "Ver ({{visibility}})",
      "tableInsert": "Añadir",
      "tableEdit": "Editar / eliminar filas propias"
    },
    "how": {
      "chat": "Chat",
      "chatApprove": "Chat, con tarjeta de aprobación",
      "workbench": "Banco de trabajo",
      "form": "Formulario del banco de trabajo",
      "web": "Versión web"
    },
    "noOps": "Esta aplicación no lee ni escribe ningún dato."
  }
} satisfies Messages<typeof zhNs>;
