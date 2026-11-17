// Editor de aplicativos: pré-visualização em tempo real, geração a partir de insumos, uso por aplicativos e importação/exportação (M5).
import type zhNs from "../zh/studio";
import type { Messages } from "../types";

export default {
  studio: {
    previewTitle: "Pré-visualização em tempo real",
    previewHint: "Busca dados como você; o visual é o mesmo da barra lateral do DSH e da versão web. Os botões não enviam nada de verdade para a IA nem gravam dados.",
    refresh: "Atualizar dados",
    untitled: "Aplicativo sem título",
    noWorkbench: "Ainda não há área de trabalho. Insira um exemplo em «Área de trabalho» à esquerda ou copie um modelo.",
    webPane: "Este painel abre a página web no DSH:",
    docPane: "O texto original da base de conhecimento referenciada será aberto neste painel.",
    previewSend: "No DSH, este botão envia o texto abaixo para a IA:",
    previewInsert: "No DSH e na versão web, este formulário grava diretamente na tabela «{{table}}»; na pré-visualização nada é gravado.",
    draftNotes: "O rascunho já foi gerado a partir dos insumos; revise antes de salvar:",
    copyAsMine: "Copiar como meu aplicativo",
    allKnowledgeBases: "All knowledge bases",
    kbItem: "“{{title}}”",
    kbSeparator: ", ",
    makeAppFromConn: "Criar aplicativo com esta conexão",
    makeAppFromKB: "Criar aplicativo de perguntas e respostas com esta base",
    export: "Exportar",
    exportApp: "Exportar arquivo do aplicativo",
    exportWithDocs: "Exportar (com documentos da base de conhecimento)",
    usedBy: "Usado por estes aplicativos ({{count}})",
    usedByEmpty: "Nenhum aplicativo usa isto ainda.",
    import: "Importar aplicativo",
    importBadFile: "Este arquivo não é JSON. Selecione um arquivo de aplicativo exportado pelo dsh-ee.",
    importFailed: "Falha na importação",
    importDone: "Importado como /{{slug}}",
    importDesc: "Após importar, apenas você pode usá-lo; compartilhe o escopo de uso quando todas as dependências estiverem disponíveis.",
    importResult: {
      created: "Criado",
      reused: "Reutilizado",
      missing: "Ausente",
      skipped: "Ignorado",
    },
    importNoDeps: "Este aplicativo não tem dependências.",
    importRemoved: "{{count}} componentes da área de trabalho usavam conexões ou conjuntos de dados ausentes e foram removidos; após completá-los, você pode importar novamente do arquivo original.",
    close: "Fechar",
    openImported: "Ver",
  },
} satisfies Messages<typeof zhNs>;
