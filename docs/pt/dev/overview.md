# Por onde começar: três caminhos de desenvolvimento

## Três caminhos

O «desenvolvimento» no dsh-ee se divide em três caminhos conforme o entregável: escrever um plugin DSH, escrever uma extensão do dsh-ee e conectar um cliente que não é DSH. Os três compartilham a mesma identidade corporativa e o mesmo token, mas o código, as verificações e as entradas de publicação são diferentes.

| Caminho | O que você entrega | Onde isso fica no sistema | Ferramenta inicial |
| --- | --- | --- | --- |
| Autor de plugin DSH | Um pacote de plugin DSH (ferramentas, cliente, serviços, comandos etc.) | Fonte npm da empresa, pré-verificação de publicação, revisão, distribuição | Primeiro `dsh plugin add dsh-primer`; depois `dsh-primer new` para criar o scaffold e `dsh-primer check` para a autoverificação |
| Autor de extensão do dsh-ee | Uma extensão: declarativa / de serviço / local (a integrada é implementada pelo próprio dsh-ee) | Conexões da empresa, campos e regras de acesso, skills do agente | `dsh-ee extension init` cria o scaffold e `dsh-ee extension check` faz a autoverificação |
| Cliente que não é DSH | Um cliente que chama modelos e MCP via HTTP | Meus tokens, gateway de modelos, `/v1`, `/mcp` e endpoints de descoberta | Primeiro crie o token em «Meus tokens» na Área de trabalho; o caminho está em [Conectar um Harness de terceiros](/docs/harness-quickstart) |

A extensão em si tem quatro formatos (declarativa / de serviço / local / integrada), e o manifesto só pode declarar os três primeiros valores de `kind`; os detalhes estão em [Escreva uma extensão do dsh-ee](/docs/extension-quickstart).

## Antes de tudo: plugin não é extensão

**Um plugin DSH** é instalado no cliente DSH, compilado e validado pelo autor na própria máquina; o dsh-ee cuida da admissão, da hospedagem e da distribuição do lado da empresa.

**Uma extensão do dsh-ee** é a unidade de integração que se conecta ao servidor do dsh-ee, gerenciada pela CLI do próprio dsh-ee e configurada pelo administrador em «Conexões da empresa» e «Campos e regras de acesso».

| Item de comparação | Plugin DSH | Extensão do dsh-ee |
| --- | --- | --- |
| Onde executa | Na máquina do cliente DSH | Envia requisições dentro do processo do dsh-ee (declarativa), em um serviço próprio da empresa (de serviço), no computador do funcionário (local) |
| Arquivo de manifesto | `package.json` e `cordis.patch.yml` | `dsh-ee.extension.json` |
| Entrada de publicação | Fonte npm da empresa ou «Compartilhar plugin» na Área de trabalho | Upload de zip para a declarativa; o administrador informa o endereço do serviço na extensão de serviço; a local entrega pacote npm e manifesto |
| Quem pode instalar | Funcionários com a permissão «Instalar capacidades» | Configurado pelo administrador nas conexões e regras |

## Ordem de leitura sugerida

1. Primeira publicação de plugin: [Escreva um plugin DSH e publique-o no registro da empresa](/docs/plugin-quickstart)
2. Primeira extensão: [Escreva uma extensão do dsh-ee: manifesto, verificações e chamadas](/docs/extension-quickstart)
3. Para conectar um Harness de terceiros ou um cliente próprio: [Conecte um Harness de terceiros ou seu próprio cliente](/docs/harness-quickstart)
4. Para confirmar os limites de permissão: [Permissões e regras de acesso: referência rápida](/docs/access-rules)
5. Para saber o que será perguntado antes de enviar: [Entradas de publicação e como ler a pré-verificação](/docs/publish-precheck)
6. Para entender a fronteira entre normas e verificações: [Normas e verificações: ENTRY / FLOW / BOOST](/docs/quality-standards)

## O que mais é preciso preparar

| Item | Descrição |
| --- | --- |
| Conta e permissões | Publicar plugins exige «Publicar capacidades» (`registry.publish`); o material de desenvolvimento de extensões exige «Desenvolver extensões» (`extensions.develop`) |
| Endereço da fonte npm da empresa | Obtenha o endereço da fonte npm, a configuração do token e o uso de `setup.sh` no «Guia de conexão» |
| Toolchain local | Node ≥24.9 (exigência de `engines` de `dsh-primer` e `dsh-standards`) e pnpm; só extensões de serviço exigem Go 1.27+ |
| Rede | Em implantações em rede interna, o GitHub e o npm público podem estar inacessíveis; confirme primeiro que consegue obter as dependências e, se necessário, use o «pré-carregamento upstream» |

## Introdução neste site e textos do repositório

A página do site traz apenas o resumo e os pontos de entrada na visão do dsh-ee, sem copiar o texto upstream; os textos do repositório podem ser citados ou adaptados conforme a licença de cada um (CC BY 4.0 mantém TASL e Change Notice; Apache-2.0 mantém LICENSE/NOTICE), e o texto, a licença e a versão upstream prevalecem nos documentos do repositório e no repositório upstream (os links apontam para os documentos após o merge):

| Conteúdo | Onde |
| --- | --- |
| Índice da documentação de desenvolvimento (Início rápido / Extensões / Harness / Publicação / Normas / Erros e FAQ) | [docs/dev/README.md (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/README.md) |
| Guia de desenvolvimento de plugins DSH (camada de referência local do guia da comunidade) | [docs/plugin-dev/README.md (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md) |
| Referência para autores de normas e checker (ENTRY / FLOW / BOOST, DS-C, checker) | [docs/plugin-dev/standards/README.md (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) |
| Visão geral do protocolo de extensões v1 | [docs/extension-protocol/README.md (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md) |
| Publicação, revisão e permissões da empresa | [docs/enterprise-publishing.md (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md) |

Versões fixas das ferramentas upstream: `dsh-primer` ([página do pacote npm](https://www.npmjs.com/package/dsh-primer)) e `dsh-standards@1.6.9` ([npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)); o dsh-ee apenas fixa as versões e indica o uso, sem reproduzir a lógica de verificação do upstream.
