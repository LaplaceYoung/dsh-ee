# Entradas de publicação e como ler a pré-verificação

## Três entradas de publicação

| Entrada | Caminho | Cenário indicado | Quando a pré-verificação roda |
| --- | --- | --- | --- |
| Envio pela web | «Compartilhar plugin» na Área de trabalho | Primeira publicação, para ajustar enquanto acompanha a pré-verificação | Roda logo após o envio |
| Publicação direta no npm | `npm publish --registry <fonte da empresa>` | Com CI existente, publicação pela pipeline de versão | Roda na publicação |
| API do agente | `POST /api/agent/packages` | Publicação automatizada e migração em lote | Roda na publicação |

As três entradas seguem o mesmo conjunto de regras de admissão: o pacote passa primeiro pela pré-verificação estática, depois pela checagem de conflito de publicação e, por fim, o modo de admissão decide se entra no ar direto ou vai para revisão.

## O que a pré-verificação analisa

A pré-verificação apenas descompacta e faz uma verificação estática: **não executa o código do plugin**, não grava no banco e não reserva o número de versão. O resultado tem três níveis: pass / warn / fail.

| Chave de verificação | O que observa | Severidade |
| --- | --- | --- |
| `bundle` | `dsh.bundle` e arquivo de patch: ausência, caminho inseguro, arquivo fora do pacote | fail para pacote de plugin; aviso quando falta em pacote que não é plugin |
| `patch` | Estrutura do `cordis.patch.yml` (array, `insert`, `id`/`name`, ids duplicados) | erro de estrutura é fail; nomes inconsistentes etc. são aviso |
| `cordis` | Se pacotes relacionados ao Cordis entraram em `dependencies` | Aviso |
| `cordis-peer` | Se o intervalo de Cordis em `peerDependencies` se afasta do valor recomendado | Aviso |
| `skills` | Lista de skills distribuídas com o pacote | Aviso (indica que a revisão vai ver) |
| `install-scripts` | Scripts de instalação (`preinstall` etc.) | Aviso |
| `secrets` | Se há suspeita de segredo dentro do pacote | **fail** |
| `license` | Se declara licença SPDX | **Aviso** (não bloqueia) |
| `engines` | Se o plugin declara `engines.dsh` | Aviso |
| `dependencies` | Lista de dependências fora do escopo da empresa | Informação |
| `size` | Tamanho do pacote (aviso acima de 20 MB) | Aviso |

A pré-verificação só responde «se a empresa pode receber este pacote com segurança», não avalia qualidade nem classifica ou ordena plugins. Normas e checkers fazem parte da toolchain upstream, e o resultado é material de comprovação do autor, que não se junta à pré-verificação da empresa.

## Como ler o resultado

A pré-verificação retorna três campos de conclusão, que correspondem ao comportamento real da API de publicação:

| Campo de conclusão | Significado |
| --- | --- |
| `can_submit` | Sem conflito de publicação; a API de publicação aceita o pacote |
| `will_publish` | No modo aberto, a verificação não tem itens fail e a publicação entra no ar imediatamente |
| `requires_review` | Pode ser enviado, mas entra na fila de revisão |

O que realmente decide «entrar no ar direto ou ir para revisão» são o modo de admissão e os itens fail:

| Situação | Modo de lista de permissões (padrão) | Modo aberto |
| --- | --- | --- |
| Conflito de publicação (nome do pacote / versão reservada / escopo / propriedade / pacote upstream de mesmo nome) | A API de publicação recusa | Igual |
| Com itens fail (como secrets, erro de estrutura do bundle) | Pode enviar e entra na fila de revisão; **não é recusa** | Pode enviar e entra na fila de revisão |
| Só avisos, ou tudo pass | Entra na fila de revisão | Sem itens fail, a publicação entra no ar imediatamente |

O número de versão é reservado no momento em que o envio é aceito; versões rejeitadas ou despublicadas também não podem ser reenviadas — corrija e aumente o número de versão.

## Autoverificação antes de enviar

1. Empacote depois de `pnpm build` e confirme que a lista `files` contém todos os artefatos.
2. Rode `dsh-primer check` e `npx dsh-standards@1.6.9 check .` localmente e deixe o resultado no CI do repositório do autor.
3. O pacote não deve conter `.tgz`, source maps, chaves reais em fixtures de teste nem caminhos absolutos da máquina.
4. Declare `license`, `engines.dsh` e a linha de versão DSH para não descobrir a incompatibilidade só na instalação.
5. Se precisar de scripts de instalação, explique na descrição do envio o que o script faz.

## Perguntas frequentes

| Sintoma | Causa comum | Tratamento |
| --- | --- | --- |
| A pré-verificação mostra conflito de publicação | Nome do pacote fora do escopo da empresa, versão já existente, pacote upstream de mesmo nome | Esses conflitos fazem a API de publicação recusar diretamente; altere o nome do pacote ou aumente a versão |
| A pré-verificação tem itens fail | Erro de estrutura do bundle/patch ou suspeita de segredo | Corrija antes de enviar; é possível enviar com itens fail, mas vai para revisão e o número de versão fica reservado |
| Após o envio, entra na fila de revisão | Modo de lista de permissões, ou itens fail / avisos no modo aberto | Corrija conforme os comentários da revisão; não repita o mesmo problema com novos números de versão |
| A revisão não conclui | Scripts de instalação, dependências ou conteúdo sensível exigem confirmação manual | Explique melhor na descrição ou contate o administrador |
| Depois de instalado na empresa, a versão está errada | Cache do cliente ou versão antiga especificada | Confira a versão em `dsh plugin add <pacote>@<versão>` |

## Textos completos no repositório

Esta página do site é apenas uma introdução; o texto completo de publicação, revisão e permissões está na documentação do repositório (os links apontam para os documentos após o merge):

- [Publicação de plugins DSH (docs/dev/plugin-publish.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Publicação, revisão e permissões da empresa (docs/enterprise-publishing.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Guia do autor de plugins (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [Mapeamento entre normas e critérios de admissão (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Erros e FAQ (docs/dev/errors-faq.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
