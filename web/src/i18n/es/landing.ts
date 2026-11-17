// Página de aterrizaje
import type zhNs from "../zh/landing";
import type { Messages } from "../types";

export default {
  "audit": {
    "run": "Ejecutar",
    "install": "Instalar",
    "publish": "Enviar",
    "approve": "Aprobar",
    "yank": "Despublicar"
  },
  "landing": {
    "sources": {
      "skill": {
        "ask": "¿Se puede reembolsar esta factura de hotel de Shanghái de 1.280 CNY?",
        "answer": "Sí. El límite de alojamiento en Shanghái es de 1.300 CNY por noche y hay que adjuntar también el extracto del hotel."
      },
      "system": {
        "ask": "¿Qué clientes de la región Este vencen el mes que viene?",
        "answer": "Son 14; 3 de ellos no tienen visitas registradas en los últimos 90 días. Ya están ordenados por importe."
      },
      "data": {
        "ask": "¿Cuál es la diferencia entre GMV y volumen de negocio?",
        "answer": "El GMV incluye pedidos no pagados y cancelados; el volumen de negocio solo cuenta los pagados. Responsable de la definición: xu.ran."
      },
      "context": {
        "ask": "¿Qué permisos hay que activar la primera semana de un nuevo empleado?",
        "answer": "Son 6: GitLab, Jira, VPN, base de datos de pruebas en solo lectura, grupo de Feishu y calendario de guardias; consulte la sección 2 del manual."
      }
    },
    "hub": {
      "systems": "Sistemas actuales de la empresa",
      "sys": {
        "erp": "ERP",
        "crm": "CRM",
        "oa": "OA",
        "ticket": "Tickets",
        "dw": "Almacén de datos",
        "bi": "Informes BI"
      },
      "assets": "Activos de la empresa",
      "asset": {
        "skill": "Skill",
        "kb": "Base de conocimiento",
        "prompt": "Prompt",
        "plugin": "Plugin",
        "policy": "Política"
      },
      "connector": "MCP / OpenAPI",
      "dataset": "Conjunto de datos de la plataforma",
      "hubChip": "Centro de capacidades",
      "center": "Despliegue privado de la empresa",
      "assemble": "Ensamblar como microaplicación",
      "grant": "Revisión · Autorización",
      "apps": {
        "expense": "Asistente de gastos",
        "renewal": "Seguimiento de renovaciones",
        "metrics": "Consultas de gestión",
        "onboard": "Guía de incorporación"
      },
      "appFrom": {
        "expense": "Política + Skill",
        "renewal": "Conexión CRM",
        "metrics": "Conjunto de datos de la plataforma",
        "onboard": "Base de conocimiento del manual de I+D"
      },
      "dept": {
        "expense": "Finanzas",
        "renewal": "Ventas",
        "metrics": "Gestión",
        "onboard": "I+D"
      },
      "desc": {
        "expense": "La política de viajes se convierte en un Skill y los límites de gasto se responden al instante",
        "renewal": "CRM conectado mediante MCP; consulta de clientes con renovación según permisos",
        "metrics": "Los criterios de las métricas van al conjunto de datos de la plataforma; todos usan la misma cifra",
        "onboard": "El manual de I+D va a la base de conocimiento y las respuestas incluyen fuentes"
      },
      "pick": "Elija un caso de habilitación",
      "window": "DSH · equipo del empleado",
      "calling": "Llamando a {{pkg}}",
      "called": "{{pkg}} llamado"
    },
    "fragment": {
      "nav": {
        "overview": "Resumen",
        "apps": "Microaplicaciones",
        "market": "Mercado de capacidades",
        "setup": "Guía de integración"
      },
      "apps": "Microaplicaciones",
      "runs": "{{department}} · {{runs}} ejecuciones en los últimos 14 días",
      "openInDsh": "Abrir en DSH",
      "auditLog": "Registro de auditoría",
      "today": "Hoy",
      "os": "Sistema operativo del dispositivo",
      "devices": "{{n}} dispositivos"
    },
    "why": {
      "eyebrow": "Por qué dsh-ee",
      "title": "De las pruebas individuales a un uso controlado para toda la empresa",
      "desc": "No faltan herramientas de IA; lo que falta es llevar el conocimiento y los sistemas de la propia empresa a cada empleado de forma segura.",
      "before": {
        "tag": "Cada empleado usa herramientas de IA por su cuenta",
        "title": "Individual, puntual, sin registro",
        "knowledge": "Las políticas y los criterios de métricas los copian y pegan los empleados en el chat",
        "trust": "Plugins y Skills de origen desconocido; nadie los revisa antes de instalarlos",
        "trace": "Qué sistema llamó quién y desde qué equipo no se puede rastrear después",
        "reuse": "Los prompts y flujos útiles se quedan en el equipo de cada persona"
      },
      "after": {
        "tag": "Distribuido de forma centralizada por dsh-ee",
        "title": "Activos de la empresa, capacidad predeterminada para todos",
        "knowledge": "Políticas, sistemas y datos empaquetados como capacidades; se instalan y se preguntan directamente",
        "trust": "Cada paquete pasa primero las comprobaciones automáticas y después la aprobación del administrador y la autorización por departamento",
        "trace": "Los registros de instalación y llamadas vuelven a auditoría; si hay un problema, se puede despublicar con un clic",
        "reuse": "Los nuevos Skills que crean los empleados vuelven al ámbito de la empresa para que todos los reutilicen"
      }
    },
    "controls": {
      "xinchuang": {
        "title": "Tecnología nacional en toda la cadena",
        "desc": "El servidor, los equipos de los empleados y los modelos pueden funcionar sobre CPU, sistemas operativos y bases de datos nacionales, y con modelos nacionales como DeepSeek."
      },
      "control": {
        "title": "Controlable",
        "desc": "Despliegue privado, con soporte para funcionamiento totalmente sin conexión. Las capacidades y microaplicaciones se revisan en la entrada y se autorizan por departamento; el ámbito de la empresa nunca vuelve a consultar el origen externo."
      },
      "audit": {
        "title": "Auditable",
        "desc": "Quién instaló qué y quién llamó a qué desde qué dispositivo: los registros de ejecución de DSH vuelven a dsh-ee, de modo que todo es rastreable y se puede despublicar con un clic."
      }
    },
    "steps": {
      "deploy": {
        "title": "Despliegue",
        "desc": "docker compose o paquete de instalación sin conexión, un único binario, base de datos OceanBase o TiDB y soporte para funcionamiento totalmente sin conexión."
      },
      "capture": {
        "title": "Consolidación",
        "desc": "Cada departamento publica sus Skills, conectores y paquetes de conocimiento en el ámbito de la empresa, por ejemplo @acme."
      },
      "review": {
        "title": "Revisión y publicación",
        "desc": "Se comprueban automáticamente los scripts de instalación, las dependencias, la licencia y las fugas de secretos; después el administrador aprueba y define el alcance de visibilidad."
      },
      "use": {
        "title": "Uso por los empleados",
        "desc": "Un solo comando conecta con el origen de la empresa; a partir de ahí, DSH y las microaplicaciones del banco de trabajo se usan con normalidad."
      }
    },
    "features": {
      "open": "Expandir: {{title}}",
      "close": "Cerrar",
      "what": "Qué hace",
      "how": "Cómo se usa",
      "hub": {
        "title": "Centro de capacidades: plugins, habilidades y conexiones en un solo lugar",
        "lead": "Los plugins, las habilidades, los prompts, las conexiones de empresa y las microaplicaciones se gestionan de forma centralizada en el Centro de capacidades. Cuando el administrador los activa, se sincronizan automáticamente con el DSH de los empleados, que no tienen que buscarlos ni instalarlos por su cuenta.",
        "points": [
          "Cinco tipos de capacidades en pestañas, cada una con versión, alcance de disponibilidad y registro de llamadas",
          "Defina la visibilidad por departamento o rol y configúrela como instalación automática",
          "Desactive o despublica en cualquier momento; el lado del empleado se sincroniza"
        ],
        "steps": [
          "Abra «Centro de capacidades» y elija o busque la capacidad que necesita",
          "Defina el alcance: toda la empresa, un departamento concreto o un rol concreto",
          "Pulse «Activar» y los empleados podrán usarla la próxima vez que abran DSH"
        ],
        "cta": "Ir al Centro de capacidades",
        "ui": {
          "tabs": [
            "Plugins",
            "Habilidades",
            "Prompts",
            "Conexiones de empresa",
            "Microaplicaciones"
          ],
          "scopes": [
            "Toda la empresa",
            "Departamento de Finanzas",
            "Departamento de Ventas",
            "Centro de I+D"
          ],
          "synced": "Sincronizado en 1.497 DSH"
        }
      },
      "review": {
        "title": "Primero revisar, luego publicar",
        "lead": "Los plugins que comparten los empleados y los de la comunidad que se incorporan pasan siempre por una revisión antes de entrar en el origen de la empresa: el sistema comprueba automáticamente y el revisor contrasta los resultados; solo entonces son visibles para los empleados.",
        "points": [
          "Comprobación automática de scripts de instalación, dependencias, licencia y fugas de secretos",
          "Las habilidades, los prompts y las microaplicaciones se comparan con la versión en línea para ver los cambios",
          "Al rechazar se indica el motivo y quien lo envió puede verlo en «Mis envíos»"
        ],
        "steps": [
          "El empleado sube el .tgz en «Compartir capacidad» o ejecuta npm publish directamente",
          "El revisor consulta los resultados y los cambios en «Pendiente de revisión»",
          "Tras la aprobación entra en el ámbito de la empresa y se distribuye según el alcance de disponibilidad"
        ],
        "cta": "Ver la cola de revisión",
        "ui": {
          "checks": [
            "Sin scripts de instalación",
            "Dependencias replicadas",
            "Licencia conforme",
            "Escaneo de secretos superado"
          ],
          "reject": "Rechazar",
          "approve": "Aprobar"
        }
      },
      "connect": {
        "title": "Integración con un solo comando",
        "lead": "El empleado ejecuta un comando en su equipo y confirma el código de dispositivo en el navegador; DSH queda conectado al origen de la empresa: el origen npm, los modelos y el Centro de capacidades se configuran de una vez, y el token se guarda en el almacén de credenciales propio de DSH.",
        "points": [
          "macOS / Linux usan setup.sh; Windows usa setup.ps1",
          "El token unificado se cruza con los permisos y scopes de la cuenta; el servidor solo guarda el hash",
          "Gestión en cualquier momento desde el chat de DSH con /ee-login, /ee-status y /ee-logout"
        ],
        "steps": [
          "Copie el comando de la «Guía de integración» y ejecútelo en la terminal",
          "Abra en el navegador la dirección que se indica y confirme el código de dispositivo",
          "Vuelva a DSH: las capacidades y los modelos de la empresa ya están disponibles"
        ],
        "cta": "Ver la guía de integración",
        "ui": {
          "confirm": "Confirme el código de dispositivo en el navegador",
          "done": "Dispositivo confirmado; DSH conectado al origen de la empresa"
        }
      },
      "knowledge": {
        "title": "Preguntas y respuestas sobre la base de conocimiento, con fuentes en cada frase",
        "lead": "Suba políticas, manuales y documentación a la base de conocimiento y los empleados preguntarán directamente en «Preguntas y respuestas» de DSH. Sin depender de modelos de vectores: el agente busca con palabras clave en varias rondas, lee el texto original y responde después; cada fuente se puede abrir y lleva a las líneas exactas del original.",
        "points": [
          "Admite PDF, Word y Markdown; los documentos tienen versiones y se pueden revertir",
          "Defina por base quién puede consultar y quién la mantiene",
          "Los enlaces a las fuentes llevan al original y resaltan las líneas correspondientes"
        ],
        "steps": [
          "Cree una base de conocimiento, defina la visibilidad y suba documentos",
          "Haga una búsqueda de prueba para confirmar que se encuentra",
          "Los empleados abren «Preguntas y respuestas» en DSH y preguntan directamente"
        ],
        "cta": "Ir a las bases de conocimiento",
        "ui": {
          "ask": "En un viaje de trabajo, ¿cuál es el límite por noche de hotel?",
          "answer": "En ciudades de primer nivel, hasta 1.300 CNY por noche; en otras ciudades, hasta 800 CNY. Si se supera, se requiere la aprobación del responsable del departamento.",
          "source": "Política de viajes y gastos v3 · líneas 12–18",
          "searched": "3 rondas de búsqueda, 2 textos originales leídos"
        }
      },
      "systems": {
        "title": "Sistemas de la empresa, autorizados por herramienta",
        "lead": "Conecte sistemas internos como ERP, CRM o tickets mediante servicios MCP o interfaces OpenAPI. dsh-ee custodia las credenciales y el DSH del empleado solo recibe las herramientas autorizadas para él.",
        "points": [
          "Compatible con servicios MCP e interfaces OpenAPI",
          "Credenciales custodiadas de forma centralizada, sin distribuirlas a los equipos de los empleados",
          "Cada herramienta se autoriza por separado y todas las llamadas quedan registradas"
        ],
        "steps": [
          "Añada un servicio MCP o una dirección OpenAPI en «Conexiones de empresa»",
          "Introduzca las credenciales y marque las herramientas y los alcances de autorización que se abrirán",
          "Los empleados piden al agente que las use directamente desde DSH"
        ],
        "cta": "Ver conexiones de empresa",
        "ui": {
          "conn": "CRM (servicio MCP)",
          "vault": "Credenciales custodiadas",
          "tools": [
            "Consultar cliente",
            "Consultar historial de renovaciones",
            "Modificar importe del contrato"
          ],
          "scopes": [
            "Departamento de Ventas",
            "Departamento de Ventas · Atención al cliente",
            "Sin autorizar"
          ]
        }
      },
      "models": {
        "title": "Puerta de enlace de modelos y cuotas",
        "lead": "dsh-ee incluye una puerta de enlace compatible con OpenAI: tras conectar orígenes (upstream) como DeepSeek, expone nombres de modelo unificados, permite asociar varios orígenes a un mismo modelo con conmutación por error automática y asigna una cuota mensual de tokens a cada cuenta.",
        "points": [
          "Comprobación de conexión con el origen, importación de modelos y nombres externos",
          "Un mismo modelo con varios orígenes y conmutación automática ante errores",
          "Cuota mensual, modelos disponibles y vigencia por cuenta; al superar la cuota se devuelve 429"
        ],
        "steps": [
          "Añada orígenes e importe modelos en «Conexión de modelos»",
          "Asigne una cuota mensual a las cuentas en «Asignación de cuotas»",
          "Consulte el uso por departamento y modelo en «Datos operativos»"
        ],
        "cta": "Ver conexión de modelos",
        "ui": {
          "usage": "Tokens usados en los últimos 30 días",
          "quota": "Cuota mensual del Centro de I+D",
          "upstreams": [
            "DeepSeek (oficial)",
            "Puerta de enlace de modelos interna"
          ],
          "status": [
            "Principal",
            "Reserva"
          ]
        }
      }
    },
    "nav": {
      "how": "Cómo funciona",
      "why": "Por qué",
      "workbench": "Banco de trabajo",
      "control": "Control y auditoría",
      "deploy": "Despliegue"
    },
    "enter": "Entrar al banco de trabajo",
    "heroLine1": "Que el DSH de cada empleado conozca su empresa.",
    "heroLead": "Conecte el ERP, el CRM, el almacén de datos y la documentación de políticas; ensamble microaplicaciones de IA y, tras la revisión y autorización, llévelas al DSH de cada empleado. Los datos y las credenciales permanecen en sus propios servidores y cada llamada es auditable.",
    "tryDemo": "Probar como empleado",
    "videoLabel": "Vídeo de presentación de dsh-ee (36 s)",
    "seeHow": "Ver cómo funciona",
    "howTitle": "Cómo funciona",
    "howLine": "De los activos de la empresa a las manos de los empleados, cada paso es controlable",
    "wbLine1": "Los activos se convierten en microaplicaciones",
    "wbLine2": "y se ejecutan en DSH",
    "wbDesc": "Asistente de gastos, consultas sobre datos de gestión, revisión de contratos… La interfaz la aloja dsh-ee; la inferencia y las llamadas a herramientas las ejecuta el DSH del equipo del empleado, y las capacidades necesarias se instalan automáticamente la primera vez que se abren.",
    "toDsh": "dsh-ee aporta a DSH",
    "toDshDesc": "Skills y contexto de empresa, un origen de instalación de confianza, microaplicaciones y políticas de uso.",
    "toEe": "DSH aporta a dsh-ee",
    "toEeDesc": "El motor de ejecución, los registros de instalación y llamadas, y los nuevos Skills que crean los empleados.",
    "controlLine1": "Controlable y auditable,",
    "controlLine2": "ejecutándose en entornos nacionales",
    "deployLine1": "Despliegue en diez minutos,",
    "deployLine2": "integración con un solo comando",
    "codeServer": "# Servidor",
    "codeClient": "# Equipo del empleado; una sola integración",
    "codeUse": "# A partir de aquí, use DSH con normalidad",
    "authNote": "Compatible con cuentas locales, LDAP y OIDC; el paquete de instalación de DSH se verifica con sha256 antes de entrar en la red interna.",
    "closing": "Haga que el conocimiento de la empresa sea la capacidad predeterminada del DSH de cada empleado.",
    "footerNote": "Prototipo; todos los datos son simulados. Los datos de plugins de origen proceden de dshfind.com.",
    "footerMarket": "Mercado de capacidades",
    "footerAdmin": "Consola de administración"
  }
} satisfies Messages<typeof zhNs>;
