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
    "wireIn": "lee mediante",
    "wireOut": "ofrece",
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
  },
  "connAgent": {
    "intro": "Olvídate del formulario campo por campo: describe el sistema que quieres conectar y cómo, copia el prompt de abajo y envíaselo a la IA en DSH. Comprueba si el sistema ya está conectado, te enumera los campos para que los confirmes y luego crea la conexión; después introduces las credenciales en la página.",
    "describeTitle": "1. Describe el sistema",
    "describeLabel": "Qué conectar y para qué",
    "describePlaceholder": "p. ej. nuestro sistema de viajes: los empleados consultan sus solicitudes de viaje, envían gastos y revisan su estado; puedo pegar la especificación OpenAPI abajo",
    "examples": "Prueba con estos:",
    "example": {
      "travel": {
        "label": "Sistema de viajes",
        "text": "Nuestro sistema de viajes: consultar solicitudes de viaje, enviar gastos y revisar su estado; tenemos una especificación OpenAPI."
      },
      "crm": {
        "label": "CRM",
        "text": "Nuestro CRM: consultar clientes, historial de contactos y oportunidades abiertas; dirección de intranet, autenticación con Bearer Token."
      },
      "ticket": {
        "label": "Tickets",
        "text": "El sistema de tickets de TI: consultar tickets, filtrar por estado y crear nuevos; solo tiene un endpoint MCP."
      },
      "hr": {
        "label": "RR. HH. y vacaciones",
        "text": "El sistema de RR. HH.: consultar vacaciones restantes, horas extra y estadísticas de asistencia; que la IA me pida los detalles de la API."
      }
    },
    "wayLabel": "Cómo conectar",
    "way": {
      "openapi": "Especificación OpenAPI",
      "mcp": "Servidor MCP",
      "unsure": "No lo sé, que decida la IA"
    },
    "urlLabel": "URL del endpoint (opcional)",
    "urlPlaceholder": "p. ej. https://api.example.com o el endpoint MCP",
    "specLabel": "Especificación OpenAPI (opcional, pégala aquí)",
    "specPlaceholder": "Pega el openapi.yaml / swagger.json, o su URL",
    "authLabel": "Autenticación",
    "auth": {
      "none": "Ninguna",
      "bearer": "Bearer Token",
      "header": "Cabecera personalizada",
      "unsure": "No lo sé"
    },
    "headerLabel": "Nombre de la cabecera",
    "headerPlaceholder": "p. ej. X-Api-Key",
    "secretNote": "No pongas credenciales (tokens, claves) en el prompt ni en el chat: cuando exista la conexión, introdúcelas en su página; solo se guardan en el servidor.",
    "promptTitle": "2. Copia el prompt y envíalo a DSH",
    "promptLabel": "Prompt para DSH",
    "copy": "Copiar prompt",
    "copied": "Copiado: pégalo en una conversación nueva de DSH",
    "openDsh": "Abrir DSH",
    "emptyDesc": "Primero escribe una línea sobre el sistema.",
    "manualHint": "¿Prefieres rellenar los campos tú mismo o ajustar los detalles? Usa la configuración manual.",
    "stepsTitle": "Qué pasa después",
    "steps": [
      "La IA comprueba con ee_hub search si este sistema ya está conectado y te avisa si es así.",
      "Te enumera el nombre, el tipo de conexión, el endpoint, la autenticación y la descripción para que los confirmes; di qué cambiar hasta que esté bien.",
      "Cuando confirmes, crea la conexión con ee_hub create (puede pasar por revisión según la configuración).",
      "De vuelta en la página de la conexión, introduce las credenciales y pruébala; la lista de herramientas se sincroniza sola. Después abre el alcance a tus compañeros."
    ],
    "notReady": "¿DSH aún no está instalado o conectado?",
    "goSetup": "Ir a la guía de configuración",
    "goConns": "Ver sistemas de negocio",
    "iterateTitle": "Sigue ajustándola después",
    "iterateDesc": "Dilo en DSH; la IA la ajusta con ee_connections_admin / ee_hub_admin:",
    "iterate": [
      "desactiva las herramientas de escritura de la conexión /<slug>",
      "limita una herramienta de /<slug> solo al rol de finanzas",
      "vuelve a sincronizar la lista de herramientas de /<slug>"
    ],
    "tipsTitle": "Consejos",
    "tips": [
      "¿La especificación OpenAPI es muy larga? Pega solo los endpoints clave; la IA construye la lista de herramientas.",
      "¿No sabes cómo conectar? Escribe solo el nombre del sistema; la IA lo comprueba primero y luego te pregunta.",
      "Mantén las credenciales y los dominios de intranet fuera de modelos externos; el prompt de esta página se queda en tu propio DSH."
    ],
    "prompt": {
      "head": "Ayúdame a conectar un sistema de negocio en dsh-ee. Prepara solo la configuración; yo introduciré las credenciales en la página.",
      "what": "[Sistema]",
      "wayOpenapi": "[Cómo conectar] Hay especificación OpenAPI",
      "wayMcp": "[Cómo conectar] Servidor MCP",
      "wayUnsure": "[Cómo conectar] Aún sin decidir: ayúdame a elegir entre OpenAPI y MCP y pídeme lo que falte.",
      "url": "[Endpoint] {{url}}",
      "doc": "[Especificación OpenAPI]\n{{doc}}",
      "auth": "[Autenticación] {{auth}}{{header}} (no pongas credenciales en spec; las introduciré en la página)",
      "steps": "Cómo:\n1. Primero usa ee_hub search (kind: connection) para comprobar que este sistema no está ya conectado; avísame si lo está.\n2. Enumera el nombre, slug, tipo, endpoint, autenticación e instrucciones de la conexión para que los confirme; no envíes nada antes de que confirme.\n3. Cuando confirme, usa ee_hub create con kind: connection: spec lleva title, description, type (openapi o mcp), url, auth, instructions (qué hace este sistema y cuándo lo usan los empleados) y openapi si se dio una especificación. NO incluyas ningún secreto.\n4. Al terminar, dame la página de detalle {{origin}}/connections/<slug>; si tienes herramientas de administración, pruébala con ee_connections_admin test y enumera las herramientas descubiertas."
    }
  }
} satisfies Messages<typeof zhNs>;
