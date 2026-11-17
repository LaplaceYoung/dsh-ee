/** «Empezar»: pega un mensaje en DSH y conecta automáticamente el banco de trabajo de la empresa. */
import type zhNs from "../zh/get-started";
import type { Messages } from "../types";

export default {
  "getStarted": {
    "title": "Empezar con {{org}}",
    "subtitle": "Pega el mensaje de abajo en DSH. Instala el plugin de la empresa, te pide iniciar sesión una vez y luego te guía por los modelos, la cuota y los skills que puedes usar.",
    "later": "Ahora no",
    "enter": "Ir al banco de trabajo",
    "other": "¿Prefieres la terminal?",
    "otherLink": "Usar el comando de una línea",
    "retry": "Reintentar",
    "step1": "① Envía esto en DSH",
    "pathPrompt": "DSH ya puede chatear",
    "pathManual": "DSH recién instalado",
    "promptDesc": "Copia esto y envíalo en un chat nuevo de DSH. Después de instalar el plugin de la empresa, DSH te pedirá que introduzcas un comando de inicio de sesión.",
    "manualAdd": "En DSH, abre Plugins → Añadir plugin y pega esta dirección:",
    "manualRestart": "Reinicia DSH cuando termine la instalación.",
    "manualLogin": "Escribe esto en el cuadro de chat:",
    "noDsh": "¿Aún no tienes DSH?",
    "download": "Descargar la aplicación de escritorio",
    "step2": "② Confirma el inicio de sesión aquí",
    "waiting": "Esperando a que DSH inicie el inicio de sesión… (código de configuración {{code}}, válido hasta {{time}})",
    "expired": "El código de configuración ha caducado.",
    "regenerate": "Crear uno nuevo",
    "pending": "«{{client}}» ({{os}}, {{ip}}) quiere iniciar sesión como tú. Código de confirmación:",
    "pendingHint": "Asegúrate de que coincide con el código que muestra DSH antes de confirmar. Deniega si no has iniciado esto.",
    "approve": "Autorizar",
    "deny": "Denegar",
    "approved": "Autorizado. Esperando a que DSH termine de iniciar sesión…",
    "denied": "Denegado. Para volver a iniciar sesión, introduce {{cmd}} en DSH.",
    "connected": "«{{client}}» está conectado",
    "step3Admin": "③ Activar el modo de administración",
    "adminDesc": "Para cambiar la configuración del sistema, el agente de DSH necesita el modo de administración temporal en este DSH. El agente no puede activarlo por sí mismo: introduce tu contraseña aquí.",
    "password": "Contraseña",
    "hours": "{{n}} h",
    "adminOnButton": "Activar el modo de administración",
    "adminOn": "El modo de administración está activado",
    "step3": "③ Continuar en DSH",
    "step4Admin": "④ Continuar en DSH",
    "nextDesc": "Envía esto en DSH. El agente te presentará tus modelos, tu cuota y los skills recomendados, y activará los que quieras.",
    "nextDescAdmin": "Envía esto en DSH. El agente te guiará por la lista de comprobación de la configuración; el progreso de abajo se actualiza automáticamente."
  }
} satisfies Messages<typeof zhNs>;
