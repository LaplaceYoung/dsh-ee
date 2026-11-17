/** Textos del modo de incrustación en iframe del mismo sitio (?embed=1). */
import type zhNs from "../zh/embed";
import type { Messages } from "../types";

export default {
  "embed": {
    "loggedOut": "Esta sesión incrustada se ha cerrado. El host puede volver a abrir la aplicación o iniciar sesión en un navegador.",
    "authRequired": "Tu sesión ha caducado o no has iniciado sesión. Vuelve a abrir esta aplicación desde la página host, o ábrela en un navegador e inicia sesión allí.",
    "connecting": "Conectando a través de la página host…",
    "connectHint": "El host completará la autorización automáticamente. Si el problema persiste, abre la aplicación en el nivel superior de un navegador.",
    "openInBrowser": "Abrir en el navegador",
    "failureHint": "El modo incrustado nunca redirige a la página de inicio de sesión dentro del iframe. El host puede escuchar dsh-ee:error, iniciar la sesión del usuario en el nivel superior y volver a abrir la aplicación.",
    "mobileHint": "Estás viendo esta aplicación en una incrustación móvil. El contenido sigue funcionando; ábrela en el nivel superior de un navegador para tener la experiencia completa.",
    "exchangeNetwork": "Can't reach dsh-ee",
    "exchangeInvalidCode": "Invalid sign-in code"
  }
} satisfies Messages<typeof zhNs>;
