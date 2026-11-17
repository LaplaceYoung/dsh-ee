/** Editor de aplicaciones: vista previa en vivo, generación de componentes, quién la usa e importación/exportación (M5). */
import type zhNs from "../zh/studio";
import type { Messages } from "../types";

export default {
  "studio": {
    "previewTitle": "Vista previa en vivo",
    "previewHint": "Los datos se obtienen con tu identidad y se ven igual que en la barra lateral de DSH y en la web. Aquí los botones no hablan realmente con la IA ni escriben datos.",
    "refresh": "Actualizar datos",
    "untitled": "Aplicación sin título",
    "noWorkbench": "Aún no hay banco de trabajo. Inserta el ejemplo en «Banco de trabajo» a la izquierda, o copia una plantilla.",
    "webPane": "En DSH, este panel abre una página web:",
    "docPane": "Los documentos citados de la base de conocimiento se abren en este panel.",
    "previewSend": "En DSH, este botón envía lo siguiente a la IA:",
    "previewInsert": "En DSH y en la web, este formulario escribe directamente en la tabla «{{table}}»; la vista previa no escribe.",
    "draftNotes": "Se ha preparado un borrador a partir de la fuente. Revísalo antes de guardar:",
    "copyAsMine": "Copiar como mi aplicación",
    "allKnowledgeBases": "All knowledge bases",
    "kbItem": "“{{title}}”",
    "kbSeparator": ", ",
    "makeAppFromConn": "Crear una aplicación con esta conexión",
    "makeAppFromKB": "Crear una aplicación de preguntas y respuestas a partir de esta base",
    "export": "Exportar",
    "exportApp": "Exportar archivo de aplicación",
    "exportWithDocs": "Exportar con los documentos de la base de conocimiento",
    "usedBy": "Usada por estas aplicaciones ({{count}})",
    "usedByEmpty": "Aún no la usa ninguna aplicación.",
    "import": "Importar aplicación",
    "importBadFile": "Ese archivo no es JSON. Elige un archivo de aplicación exportado desde dsh-ee.",
    "importFailed": "Error al importar",
    "importDone": "Importada como /{{slug}}",
    "importDesc": "Por ahora solo puedes usarla tú. Compártela cuando sus dependencias estén listas.",
    "importResult": {
      "created": "Creada",
      "reused": "Reutilizada",
      "missing": "Faltante",
      "skipped": "Omitida"
    },
    "importNoDeps": "Esta aplicación no tiene dependencias.",
    "importRemoved": "{{count}} widgets del banco de trabajo usaban una conexión o un conjunto de datos que falta y se han eliminado. Vuelve a importar el archivo original cuando existan.",
    "close": "Cerrar",
    "openImported": "Abrirla"
  }
} satisfies Messages<typeof zhNs>;
