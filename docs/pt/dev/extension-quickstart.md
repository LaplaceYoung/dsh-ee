# Escreva uma extensão do dsh-ee: manifesto, verificações e chamadas

## Quatro formatos

Extensões do dsh-ee conectam sistemas da empresa. Escolha o formato primeiro: os três primeiros são entregues por manifesto e o quarto é a integração oficial implementada pelo próprio dsh-ee, para a qual o autor não escreve manifesto.

| Formato | Manifesto `kind` | Onde executa | Entregável | Como o dsh-ee chama |
| --- | --- | --- | --- | --- |
| Declarativa | `declarative` | Envia HTTP dentro do processo do dsh-ee | Manifesto + documento OpenAPI (zip) | Gera ferramentas a partir do OpenAPI e chama o sistema de origem diretamente |
| De serviço | `service` | Serviço HTTP implantado pela própria empresa | Um serviço que implementa o manifesto e a interface acordada | Busca `/.well-known/dsh-ee-extension.json`; as ferramentas passam por `POST /mcp` |
| Local | `local` | Pacote npm no computador do funcionário | Pacote npm + manifesto | O agente se conecta ao MCP local; antes de uma operação de escrita pede confirmação ao dsh-ee e, após a chamada, reporta um resumo |
| Integrada | (sem manifesto) | Go dentro do processo do dsh-ee | Acompanha a atualização do dsh-ee | Mesma interface interna das extensões de serviço, sem tráfego de rede (por exemplo, a fonte de conhecimento do Feishu Docs) |

O schema do manifesto só aceita os três valores de `kind`: `declarative` / `service` / `local`; a «integrada» é implementada pelo próprio dsh-ee e não pode ser entregue por manifesto. O servidor não executa código de terceiros: na declarativa é o dsh-ee que faz a requisição, a extensão de serviço fica no serviço da própria empresa e a local fica no computador do funcionário.

## Criar um scaffold

```bash
# 声明式：不写代码，有 OpenAPI 就能接
dsh-ee extension init ./my-ext --id acme/my-ext --kind declarative

# 服务式：要写逻辑，企业自己部署
dsh-ee extension init ./my-service --id acme/my-service --kind service
```

`init` exige `--id autor/nome` para definir a identidade; `--kind` aceita `declarative` (padrão) ou `service`. A extensão local é copiada do template ou exemplo [`extensions/official/legacy-local`](https://github.com/hikariming/dsh-ee/blob/main/extensions/official/legacy-local) no repositório. O diretório gerado vem com `AGENTS.md`, e abrir com o agente já mostra o próximo passo.

| Comando | Uso | O que faz na máquina |
| --- | --- | --- |
| `dsh-ee extension init` | Copia uma extensão de um exemplo | Escreve arquivos |
| `dsh-ee schema list` / `schema get manifest` | Consulta offline de especificações como manifesto e códigos de erro | Somente leitura |
| `dsh-ee extension check` | Verificação estática de manifesto, documento de API, marcação de somente leitura e forma das credenciais | Verificação somente leitura, sem executar código |
| `dsh-ee extension test` | Inicia a extensão de serviço e testa item a item conforme o protocolo | Inicia um processo local |
| `dsh-ee extension dev` | Inicia um dsh-ee simulado (sem banco de dados) | Inicia um processo local |
| `dsh-ee extension call` | Chama uma ferramenta da extensão como um funcionário | Diagnóstico somente leitura |
| `dsh-ee extension pack` | Empacota a extensão **declarativa** em zip e calcula o checksum | Escreve o zip |
| `dsh-ee extension report` | Gera um resumo de entrega para o administrador | Escreve arquivos |

`check` sai com código 1 quando há erros, e cada resultado traz `code`, `severity`, `where`, `message` e `fix`. Todos os comandos acima rodam na máquina ou na CI do repositório do autor; o servidor do dsh-ee não executa código de terceiros nem roda checkers na web.

## Exemplo mínimo de manifesto

O manifesto abaixo tem a mesma forma gerada por `dsh-ee extension init --kind declarative`; com um `openapi.yaml` coberto pelos `servers`, o resultado de `dsh-ee extension check` é 0 erros e 0 avisos. Os campos prevalecem conforme `dsh-ee schema get manifest`; o manifesto é um schema estrito, e um campo extra fora da especificação também gera erro.

```json
{
  "id": "acme/oa",
  "version": "0.1.0",
  "name": "Acme OA",
  "description": "查询和提交 Acme OA 的审批单。",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": ["https://oa.acme.internal"],
  "credential": { "mode": "ee_jwt" },
  "tools": [
    {
      "name": "list_items",
      "readOnly": true,
      "description": "列出当前员工的条目",
      "keep": ["items.id", "items.title", "total"]
    },
    {
      "name": "create_item",
      "description": "新建一个条目。这是写操作，调用前要员工确认。"
    }
  ]
}
```

Alguns pontos fáceis de errar:

- `id` deve ser escrito como «autor/nome», usando apenas letras minúsculas, números e hífens; `version` é uma versão semântica estrita.
- A declarativa precisa de `openapi` (caminho relativo dentro do diretório da extensão) e de pelo menos um `servers`; o `servers[0].url` do documento OpenAPI precisa estar dentro dos `servers` do manifesto.
- `tools[].name` corresponde ao `operationId` de cada interface do OpenAPI; ferramentas de consulta levam `readOnly` e operações de escrita não levam.
- O modo de credencial (`credential.mode`) só é necessário na chamada: `shared` / `user_secret` / `ee_jwt` / `oauth`; escolha o modo que o outro lado aceita.

## Confirme antes de entregar

| Item de verificação | Onde confirmar |
| --- | --- |
| Campos do manifesto e versão do protocolo | `dsh-ee schema get manifest`, `dsh-ee extension check --json` |
| Documento de API e mapeamento de ferramentas | Resultado `tools` do `check`; se operações de escrita estão marcadas com `readOnly` |
| Credencial obtida apenas do dsh-ee, fora do manifesto | Nem no manifesto nem no código deve haver chave em texto claro |
| Consistência de protocolo da API de serviço | Casos de teste do `dsh-ee extension test` |
| Empacotamento da declarativa | `dsh-ee extension pack` gera o zip; a extensão de serviço entrega repositório de código ou imagem |
| Quem pode usar quais ferramentas | O administrador configura em «Conexões da empresa» e «Campos e regras de acesso» |

## Relação com as skills do DSH

Para dar ao agente um conjunto de instruções executáveis, o dsh-ee escreveu o fluxo de desenvolvimento de extensões como uma skill (`dsh-ee-extension-dev`), distribuída ao DSH conforme a permissão «Desenvolver extensões» (`extensions.develop`). Pessoas e agentes veem os mesmos passos: a página serve para leitura humana e a skill para execução pelo agente.

## Textos completos no repositório

Esta página do site é apenas uma introdução; os detalhes de protocolo, a revisão de instalação e os códigos de erro prevalecem na documentação do repositório (os links apontam para os documentos após o merge):

- [Desenvolvimento de extensões (docs/dev/extensions.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/extensions.md)
- [Visão geral do protocolo de extensões v1 (docs/extension-protocol/README.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [Texto do protocolo de extensões (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [Publicação, revisão e permissões da empresa (docs/enterprise-publishing.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Índice do diretório de extensões e exemplos (extensions/README.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/README.md)
- [JSON Schema do manifesto (extensions/schemas/manifest.schema.json)](https://github.com/hikariming/dsh-ee/blob/main/extensions/schemas/manifest.schema.json)
- [Decisões de design (docs/plugins.md, documento de design, não é documentação de uso)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugins.md)
