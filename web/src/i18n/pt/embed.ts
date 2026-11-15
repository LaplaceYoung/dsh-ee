// Modo de incorporação em iframe no mesmo site (?embed=1).
import type zhNs from "../zh/embed";
import type { Messages } from "../types";

export default {
  embed: {
    loggedOut: "Esta sessão de incorporação foi encerrada. O host pode reabrir o aplicativo ou entrar pelo navegador.",
    authRequired: "A sessão expirou ou você ainda não entrou. Reabra este aplicativo na página do host ou abra-o no navegador e entre por lá.",
    connecting: "Conectando pela página do host…",
    connectHint: "O host concluirá a autorização automaticamente; se ficar parado aqui, abra o aplicativo na janela de nível superior do navegador.",
    openInBrowser: "Abrir no navegador",
    failureHint: "No modo de incorporação não há redirecionamento para a página de login aqui: o host pode escutar a mensagem dsh-ee:error, concluir o login na janela de nível superior e reabrir o aplicativo.",
    mobileHint: "Esta é a visualização de incorporação em celular; o conteúdo funciona normalmente. Para a experiência completa, abra na janela de nível superior do navegador.",
    exchangeNetwork: "Can't reach dsh-ee",
    exchangeInvalidCode: "Invalid sign-in code",
  },
} satisfies Messages<typeof zhNs>;
