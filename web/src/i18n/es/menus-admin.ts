/** Menús y páginas (administración). */
import type zhNs from "../zh/menus-admin";
import type { Messages } from "../types";

export default {
  "menusAdmin": {
    "kinds": {
      "group": "grupo",
      "page": "página",
      "link": "enlace externo"
    },
    "saved": "Guardado «{{title}}»",
    "created": "Creado «{{title}}»",
    "deleteConfirm": "¿Eliminar «{{title}}»?",
    "deleted": "Eliminado «{{title}}»",
    "editTitle": "Editar {{kind}}",
    "newTitle": "Crear {{kind}}",
    "type": "Tipo",
    "internalPage": "Página de la aplicación",
    "externalLink": "Enlace externo",
    "title": "Título",
    "titleHint": "Mientras se mantenga el título predeterminado, cada idioma muestra su propia traducción; un título que cambies se muestra tal como lo escribas.",
    "component": "Componente de página",
    "componentHint": "Qué página muestra este elemento del menú",
    "choosePage": "Elegir una página",
    "unregistered": "{{key}} (no registrada en el frontend)",
    "route": "Ruta",
    "routeHint": "Empieza por /; letras minúsculas, dígitos y - _ /. Los enlaces dentro de la aplicación se actualizan automáticamente al cambiarla.",
    "linkUrl": "URL del enlace",
    "linkHint": "Se abre en una ventana nueva",
    "group": "Grupo",
    "topLevel": "Nivel superior (sin grupo)",
    "access": "Permiso de acceso",
    "accessGroupHint": "Si se define, todo el grupo se muestra solo a quienes tengan este permiso",
    "accessPageHint": "Quien no tenga este permiso no verá el elemento del menú ni podrá abrir la página",
    "loginOnly": "Cualquier usuario con sesión iniciada",
    "icon": "Icono",
    "showInSidebar": "Mostrar en la barra lateral",
    "showInSidebarHint": "Las páginas ocultas se pueden abrir por enlace (el permiso de acceso se sigue aplicando)",
    "hidden": "Oculto",
    "unregisteredBadge": "Componente no registrado",
    "moveUp": "Subir",
    "moveDown": "Bajar",
    "resetConfirm": "¿Restaurar el menú predeterminado? Se sustituirá toda la configuración actual del menú, incluidas las páginas y los enlaces que hayas añadido.",
    "resetDone": "Menú predeterminado restaurado",
    "pageTitle": "Menús y páginas",
    "pageDesc": "Cambia el nombre de cada página, su ruta, su grupo y quién puede verla. Al guardar, la barra lateral y las rutas se actualizan de inmediato.",
    "newGroup": "Nuevo grupo",
    "newLink": "Nuevo enlace",
    "newPage": "Nueva página",
    "structure": "Estructura del menú",
    "reset": "Restaurar valores predeterminados",
    "emptyGroup": "Grupo vacío. Elígelo como grupo de una página para mover páginas aquí",
    "pickHint": "Selecciona un elemento a la izquierda para editarlo, o crea un grupo, una página o un enlace externo."
  }
} satisfies Messages<typeof zhNs>;
