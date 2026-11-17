# Conecte um Harness de terceiros ou seu próprio cliente

## Caminho de cinco minutos (H1: só `/mcp`)

Para um Harness de terceiros (Claude Code, Codex, agentes próprios, scripts) se conectar ao dsh-ee, o caminho mais curto é o **H1: um token `ee_at_` + `/mcp`**. Quando você precisar de arquivos de skill, de uma extensão local na máquina ou de um comando para gerar a configuração, siga o H2 na documentação do repositório; a integração profunda específica do DSH (H3) não é oferecida para dependência de terceiros, e você não deve projetar a integração pelo H3.

| Etapa | O que fazer | Onde ver |
| --- | --- | --- |
| 1 | Crie um token `ee_at_` em «Meus tokens», com o escopo minimizado conforme o uso | «Tokens e escopo» abaixo |
| 2 | Leia o documento de descoberta anônimo `GET /.well-known/dsh-ee` e confirme `api_version`, endpoint do código de dispositivo, capacidades e limites | «Descoberta e versão» |
| 3 | Com o escopo `models`, chame `/v1/models` e `/v1/chat/completions` | «Gateway de modelos» |
| 4 | Com o escopo `connections`, conecte o `/mcp` e faça `tools/list` dinamicamente por sessão | «MCP» |
| 5 | Ferramentas de escrita passam pela confirmação do servidor: obtenha o link «confirme na web» e, depois que o funcionário concordar, chame de novo com os mesmos parâmetros | «Operações de escrita» |

## Tokens e escopo

Os tokens começam com `ee_at_` e correspondem ao próprio usuário; o escopo é apenas um limite superior, e o servidor ainda cruza cada requisição com as permissões da conta (se a conta não tiver o ponto de permissão correspondente, o token não resolve).

| Uso | Escopo necessário | Descrição |
| --- | --- | --- |
| Chamar modelos | `models` | `/v1/models`, `/v1/chat/completions`; a conta também precisa da permissão «Chamar modelos» |
| Usar conexões da empresa, bases de conhecimento e ferramentas de dados de aplicativos via `/mcp` | `connections` | A entrada `/mcp` é verificada por `connections` |
| Usar as ferramentas `ee_*` da Área de trabalho dentro do `/mcp` | `connections` + `agent` | O interruptor geral do agente nas configurações do sistema também precisa estar ligado |
| Ler o catálogo de capacidades (opcional) | `hub` | Aplicativos e `revision` em `/v1/apps` e `/v1/capabilities` |

Token é como senha: leia de variáveis de ambiente ou de um cofre de credenciais, e não o coloque em repositórios, logs ou capturas de tela; emita um por integrador e revogue por pessoa.

## Descoberta e versão

O endpoint anônimo `GET /.well-known/dsh-ee` retorna `api_version`, o endpoint do código de dispositivo, `scopes_supported`, as capacidades de modelos/MCP/aplicativos, os limites de tamanho de requisição e a política de descontinuação. `GET /v1/capabilities` (Bearer opcional) retorna os modelos, a quantidade de ferramentas e a `revision` visíveis para o token atual; as duas respostas trazem o cabeçalho `Dsh-EE-API-Version: 1`.

O login por código de dispositivo é uma entrada pública opcional: `POST /api/device/code` retorna `device_code` / `user_code`, e, depois que o funcionário confirma na página `/device`, o cliente consulta `POST /api/device/token` no intervalo retornado como `interval`. O código de dispositivo vale por 10 minutos e o token só pode ser obtido uma vez; não há refresh token, então, se expirar, refaça o fluxo.

## Gateway de modelos (`/v1`)

Compatível com OpenAI, chamada mínima:

```sh
export BASE='https://<dsh-ee>'
export TOKEN='ee_at_…'

curl -fsS "$BASE/v1/models" -H "Authorization: Bearer $TOKEN"

curl -fsS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"<模型名>","messages":[{"role":"user","content":"你好"}]}'
```

O nome do modelo segue o que `/v1/models` retorna; streaming, limites e códigos de erro seguem a convenção de compatibilidade com OpenAI.

## MCP (`/mcp`)

O `/mcp` é Streamable HTTP + JSON-RPC 2.0: `POST /mcp` inicia a requisição com `Accept: application/json, text/event-stream`; o `Mcp-Session-Id` retornado por `initialize` vale para as requisições seguintes.

```sh
curl -fsS "$BASE/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"my-harness","version":"1.0"}}}'
```

| Prefixo da ferramenta | Origem |
| --- | --- |
| `<nome da conexão>__<nome da ferramenta>` | Ferramentas das conexões da empresa |
| `kb__*` | Ferramentas da base de conhecimento |
| `app__*` / `data__*` | A superfície de ferramentas do microapp |
| `ee_*` | Ferramentas da Área de trabalho (exigem o escopo `agent` e o interruptor geral do agente) |

As ferramentas visíveis dependem das permissões do próprio dono do token e podem variar de funcionário para funcionário: **faça `tools/list` dinamicamente em cada sessão, nunca fixe a lista**. Quando `annotations.readOnlyHint` estiver ausente na ferramenta, trate-a como operação de escrita.

## Operações de escrita: confirmação no servidor

Um Harness de terceiros, por padrão (`non_dsh`), não pode executar ferramentas de escrita diretamente: `tools/call` retorna `isError: true` e um link «confirme na web». Depois que o funcionário confere os parâmetros na página `/confirm/<id>` e concorda, é preciso chamar de novo com **exatamente os mesmos parâmetros** para a operação ser executada de fato; uma aprovação vale para uma única chamada e por 10 minutos, e, se os parâmetros mudarem, é preciso confirmar de novo. O cliente DSH pode liberar conforme o próprio interruptor de confirmação, mas não presuma que um Harness de terceiros tenha esse atalho.

## Limites

| Limite | Descrição |
| --- | --- |
| O H1 não cobre | Catálogo de skills, Área de trabalho, páginas de microapp, instalação e atualização automática de plugins fazem parte da integração profunda exclusiva do DSH; não presuma que um Harness de terceiros tenha isso |
| Tamanho da requisição | `max_request_bytes` / `max_response_bytes` conforme o documento de descoberta |
| Ciclo de vida do token | Não há refresh token: se expirar, emita outro; o fluxo de código de dispositivo só permite obter o token uma vez |
| Escopo e permissões | O escopo é o limite superior; vale «permissões da conta ∩ escopo» |
| Estabilidade da API | Os formatos existentes de `/v1`, `/.well-known/dsh-ee` e `/mcp` não mudam; novas capacidades seguem o documento de descoberta |
| Operações de escrita | Sem `readOnlyHint`, trate sempre como escrita e siga o fluxo de confirmação |

## Textos completos no repositório

Esta página do site é apenas uma introdução; os detalhes de protocolo e endpoints estão na documentação do repositório (os documentos marcados com «disponível após o merge» entram pelo PR de documentação correspondente):

- [Integração com Harness de terceiros (docs/harness-integration.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/harness-integration.md)
- [Integração de clientes que não são DSH com o L1 (docs/l1-client.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- [Descoberta e capacidades (docs/l1-client.md §2, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md#2-%E5%8F%91%E7%8E%B0%E4%B8%8E%E8%83%BD%E5%8A%9B%E4%B8%AD%E7%AB%8B%E5%A5%91%E7%BA%A6-v1)
- [A página de Harness na documentação de desenvolvimento (docs/dev/harness.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- [Visão geral do protocolo de extensões (docs/extension-protocol/README.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
