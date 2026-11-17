/** Inicio de sesión y cuenta. */
import type zhNs from "../zh/auth";
import type { Messages } from "../types";

export default {
  "login": {
    "title": "Inicia sesión en tu banco de trabajo",
    "subtitle": "Usa la cuenta que tu administrador creó para ti.",
    "username": "Nombre de usuario",
    "password": "Contraseña",
    "submit": "Iniciar sesión",
    "submitting": "Iniciando sesión…",
    "forgot": "¿Has olvidado tu contraseña? Pide a un administrador que la restablezca.",
    "ldapHint": "Puedes iniciar sesión con la cuenta de dominio de la empresa (la misma contraseña que la del equipo).",
    "ldapForgot": "Recupera tu contraseña de dominio en el sistema de cuentas de la empresa; para una cuenta local, pide ayuda a un administrador.",
    "reauth": "Se inició sesión automáticamente desde DSH con acceso solo de empleado. Inicia sesión con tu contraseña para usar todas las funciones.",
    "handoffExpired": "El enlace de inicio de sesión de DSH ha caducado (funciona una vez, dentro de un minuto). Inicia sesión para continuar.",
    "handoffOtherSession": "This browser is already signed in with another account, so the sign-in link from DSH won't replace the existing session. Sign out of the current account first, then open the link from DSH again.",
    "demo": {
      "title": "Esta es una demo pública",
      "account": "Usuario {{username}} · Contraseña {{password}}",
      "reset": "Los datos se restablecen {{reset}}, así que prueba lo que quieras.",
      "fill": "Usar cuenta de demo",
      "heroTitle": "Prueba dsh-ee como empleado de Xinghe Tech",
      "heroSubtitle": "Xinghe Tech es una empresa de software de ejemplo de 200 empleados, con CRM, gastos de viaje, tickets de TI y bases de conocimiento de 6 departamentos conectados. Elige a una persona para ver las aplicaciones de IA que usa a diario.",
      "uses": "Usa: {{app}}",
      "enterAs": "Continuar como {{name}}",
      "scenarios": {
        "pick": "Pick a demo scenario",
        "xinghe": {
          "tab": "Company · Xinghe Tech"
        },
        "novel": {
          "tab": "Small team · Shanhai Studio",
          "heroTitle": "Try dsh-ee as a member of Shanhai Studio",
          "heroSubtitle": "Shanhai Studio is a 4-person team serialising a novel, with no business systems to connect: the story bible and writing guide are knowledge bases, the chapter tracker is a table everyone can edit, and manuscript review goes to two experts. Pick a person to see the app they use every day."
        }
      },
      "adminDivider": "O inicia sesión como administrador para ver cómo se conectan, revisan, autorizan y auditan las aplicaciones",
      "dsh": {
        "title": "¿Ya tienes DSH? Conéctalo a la demo y pregunta de verdad",
        "intro": "Usa el modelo de tu propio DSH para llamar a las aplicaciones, las bases de conocimiento y los sistemas de negocio de ejemplo de la demo.",
        "step1": "Ejecuta en una terminal:",
        "unix": "macOS / Linux",
        "win": "Windows",
        "step2": "Cuando se abra la página de confirmación en tu navegador, elige un empleado en esta página para iniciar sesión y haz clic en Aprobar.",
        "step3": "De vuelta en DSH, escribe el nombre de una aplicación como /customer-brief y haz la pregunta de las tarjetas de arriba.",
        "note": "La demo no proporciona un modelo: DSH usa el modelo que ya tienes configurado. Los datos se restablecen con regularidad; vuelve a ejecutar el comando de conexión después de cada restablecimiento."
      }
    }
  },
  "account": {
    "title": "Cuenta",
    "logout": "Cerrar sesión",
    "username": "Nombre de usuario",
    "displayName": "Nombre para mostrar",
    "department": "Departamento",
    "roles": "Roles",
    "preferences": "Preferences",
    "preferencesHint": "Language and theme are saved on this device only.",
    "theme": "Theme",
    "changePassword": "Cambiar contraseña",
    "currentPassword": "Contraseña actual",
    "newPassword": "Nueva contraseña",
    "newPasswordHint": "Al menos 10 caracteres. Después tendrás que iniciar sesión de nuevo en todos los dispositivos.",
    "confirmPassword": "Repite la nueva contraseña",
    "mismatch": "Las contraseñas nuevas no coinciden",
    "source": "Origen de la cuenta",
    "sourceLocal": "Cuenta local",
    "sourceLdap": "Directorio empresarial (LDAP)",
    "email": "Correo electrónico",
    "ldapManaged": "Tu cuenta la gestiona el directorio empresarial. Cambia tu contraseña en el sistema de cuentas de la empresa; el nombre para mostrar y el departamento se sincronizan desde el directorio en cada inicio de sesión.",
    "language": "Language",
    "languageHint": "Used for the web interface and messages such as Feishu notifications. Changes apply immediately and sync to your account.",
    "followOrg": "Follow organization default"
  }
} satisfies Messages<typeof zhNs>;
