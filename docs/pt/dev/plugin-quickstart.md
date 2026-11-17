# Escreva um plugin DSH e publique-o no registro da empresa

## Pré-requisitos

| Dependência | Versão | Por que é necessária |
| --- | --- | --- |
| Node | ≥ 24.9.0 | Os `engines` de `dsh-primer` e `dsh-standards` exigem ≥24.9; a linha DSH 0.2.2 também exige Node ≥24.9, e linhas 0.2.x anteriores rodam em 22.19+ |
| DSH | Linha 0.2.x (0.2.1–0.2.2) | Executar o plugin e `dsh plugin add`; `dsh-primer` declara suporte a `>=0.2.1-0 <0.3.0`, e `engines.dsh` deve ser preenchido com o intervalo realmente validado |
| pnpm | Qualquer versão recente | Instalar dependências e empacotar |
| dsh-primer | 0.4.x | Scaffold, verificação estática e smoke test de instalação (antigo nome `dsh-plugin-guide` 0.3.x, alias antigo da CLI `dsh-plugin-dev`) |
| Conta da empresa | Com «Publicar capacidades» (`registry.publish`) | Enviar plugins e solicitar a importação de plugins da comunidade |

## Criar um scaffold em cinco minutos

```bash
# 0. dsh-primer 是可安装的 DSH bundle，不是 DSH 自带命令；先装进你用的 profile
dsh plugin --profile web add dsh-primer

# 1. 生成骨架（上游模板，TypeScript）
dsh-primer new my-plugin

# 2. 本地装依赖并跑通
cd my-plugin
pnpm install
pnpm build

# 3. 自查包形状（静态检查，在本地执行）
dsh-primer check

# 4. 装进一个测试 profile 验证加载/卸载
dsh-primer verify
```

`dsh-primer` pertence à toolchain upstream; o dsh-ee apenas fixa a versão e indica o uso, sem reproduzir a lógica de verificação dele.

## Campos do manifesto que mais importam para a empresa

| Campo | O que escrever | Por que a empresa olha |
| --- | --- | --- |
| `name` | Nome do pacote no escopo da empresa, por exemplo `@company/hello` | Define se o roteamento para a fonte npm da empresa é possível |
| `version` | Versão semântica, uma versão por publicação | O número de versão, uma vez reservado, não é liberado; conflitos de publicação recusam o envio |
| `main` / `files` | Entrada real a publicar e lista de permissões de arquivos | Evita publicar código-fonte, testes e `.tgz` juntos |
| `cordis.patch.yml` | Declaração de patch do plugin para o Cordis | A pré-verificação confere a estrutura do bundle e do patch |
| `dsh.bundle` | Manifesto do plugin DSH (se houver) | Define se o cliente reconhece o pacote |
| `engines.dsh` | Intervalo de versões DSH realmente validado | Confere com a linha de versões DSH da empresa; sem declaração, você recebe um aviso |
| `peerDependencies` | Peers relacionados ao DSH | A pré-verificação confere se o intervalo de peers é resolvível |
| `license` | Identificador SPDX, como `Apache-2.0` | A ausência de `license` é um **aviso** na pré-verificação (não bloqueia), mas vai para a revisão manual |

## O que a autoverificação local verifica

| Comando | O que verifica | Onde executar |
| --- | --- | --- |
| `dsh-primer check` | Se a forma do pacote e as entradas estão completas | Máquina ou CI do repositório do autor |
| `dsh-primer verify` | Smoke test de instalação, ativação e desinstalação | Máquina ou CI do repositório do autor |
| `npx dsh-standards@1.6.9 check .` | Evidência estática das cláusulas das normas upstream (exige Node ≥24.9) | Máquina ou CI do repositório do autor; o resultado é material de comprovação do autor |

O servidor do dsh-ee não executa código de plugin nem esses checkers; a pré-verificação de «Compartilhar plugin» na web olha apenas a forma estática do pacote e as condições de admissão da empresa.

## Publicar na fonte da empresa

| Entrada | Como fazer | Para quem |
| --- | --- | --- |
| «Compartilhar plugin» na Área de trabalho | Envie o `.tgz` e confira o resultado da pré-verificação antes de enviar | Primeira publicação; para quem quer ver a pré-verificação antes |
| Publicação direta no npm | `npm publish --registry <fonte da empresa>`; crie o token em «Meus tokens» | Quem já tem pipeline de CI |
| API do agente | `POST /api/agent/packages`, chamado com token da empresa | Publicação automatizada |

O número de versão fica reservado durante a revisão e a publicação; versões duplicadas ou em conflito são recusadas. O resultado da pré-verificação tem três itens: `can_submit` (sem conflito de publicação, a API de publicação aceita), `will_publish` (no modo aberto, a verificação não tem itens fail e a publicação entra no ar imediatamente) e `requires_review` (entra em revisão após o envio).

## Perguntas frequentes

| Sintoma | Causa comum | Tratamento |
| --- | --- | --- |
| A pré-verificação aponta arquivo de entrada ausente | A lista `files` não inclui o artefato de build, ou `main` aponta para um arquivo não compilado | Rode `pnpm build` primeiro e depois confira `files` |
| A pré-verificação tem itens fail | Estrutura de bundle / patch incorreta, segredo detectado etc. | Corrija antes de enviar; é possível enviar com itens fail, mas o número de versão fica reservado e a revisão normalmente não passa |
| A pré-verificação só tem avisos | `license` ausente, `engines.dsh` não declarado, script de instalação etc. | Não bloqueia: no modo de lista de permissões vai para revisão manual; no modo aberto pode entrar no ar direto |
| Intervalo de peers não resolvível | Duas linhas de versão DSH incompatíveis declaradas ao mesmo tempo | Mantenha apenas uma linha de versão ou complete o intervalo para que seja resolvível |
| Erro de versão do Node na instalação | `engines.node` não corresponde ao Node do computador do funcionário | Corrija `engines.node` conforme o resultado real (exemplo: `^22.19.0 \|\| >=24.9.0`) |
| Não instala na empresa | Permissão do token insuficiente ou `.npmrc` sem a fonte da empresa | Confira a permissão «Instalar capacidades» e o `.npmrc` |

## Textos completos no repositório

Esta página do site é apenas uma introdução; o texto completo e a licença estão na documentação do repositório (os links apontam para os documentos após o merge):

- [Início rápido (docs/dev/quickstart.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/quickstart.md)
- [Publicação de plugins DSH (docs/dev/plugin-publish.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Erros e FAQ (docs/dev/errors-faq.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
- [Guia do autor de plugins (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [Guia de desenvolvimento de plugins DSH (docs/plugin-dev/README.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md)
- [Upstream: repositório dsh-primer](https://github.com/PerryLink/dsh-primer) · [página do pacote npm](https://www.npmjs.com/package/dsh-primer)
- [Upstream: dsh-plugin-guide (página do nome antigo 0.3.x)](https://github.com/PerryLink/dsh-plugin-guide)
