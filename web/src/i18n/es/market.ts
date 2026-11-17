/** Mercado de capacidades y detalle de plugins. */
import type zhNs from "../zh/market";
import type { Messages } from "../types";

export default {
  "market": {
    "title": "Mercado de capacidades",
    "desc": "{{count}} capacidades revisadas, {{internal}} de ellas compartidas por compañeros. Todo se almacena en la intranet y se instala sin conexión.",
    "setupGuide": "Guía de conexión",
    "downloadDsh": "Descargar DSH {{version}}",
    "searchPlaceholder": "Buscar por nombre, finalidad o autor",
    "searchLabel": "Buscar capacidades",
    "all": "Todas",
    "source": "Origen",
    "allSources": "Todos los orígenes",
    "internal": "Internas",
    "community": "Selección de la comunidad",
    "category": "Categoría",
    "allCategories": "Todas las categorías",
    "sort": "Ordenar",
    "sortBy": "Por {{label}}",
    "sortInstalls": "populares en la empresa",
    "sortScore": "puntuación de dshfind",
    "sortRating": "valoración de los empleados",
    "sortRecent": "actualizadas recientemente",
    "count": "Resultados: {{count}}",
    "empty": "No hay capacidades que coincidan. ¿No encuentras lo que necesitas? <link>Solicita una o comparte la tuya</link>."
  },
  "pluginDetail": {
    "notFound": "Este plugin no existe o aún no se ha aprobado.",
    "backToMarket": "Volver al hub",
    "market": "Centro de capacidades",
    "internal": "Interno · {{department}}",
    "upstream": "Upstream · incluido en dshfind",
    "featured": "Destacado",
    "reviewed": "Revisado",
    "install": "Instalar",
    "runInDsh": "Ejecutar en DSH (tras conectarse a la fuente de la empresa)",
    "discoverHint": "También puedes buscar «{{title}}» en DSH Plugins → Descubrir e instalarlo allí. ¿Aún no estás conectado? Consulta la <link>guía de conexión</link>.",
    "securityChecks": "Comprobaciones de seguridad",
    "version": "Versión",
    "author": "Autor",
    "installs": "Instalaciones en la empresa",
    "installsValue": "{{count}}",
    "updated": "Actualizado",
    "deps": "Conjunto de dependencias",
    "depsValue": "{{count}} paquetes · {{size}}",
    "offline": "Se instala sin conexión",
    "offlineNo": "No (descarga binarios de internet)",
    "repo": "Repositorio de código"
  }
} satisfies Messages<typeof zhNs>;
