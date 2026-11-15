# Normas e verificações: ENTRY / FLOW / BOOST

## O que cada uma das três camadas de normas responde

As «normas» vêm do upstream dsh-plugin-guide / dsh-primer e do dsh-standards. O dsh-ee **não executa o checker no servidor**; a documentação do repositório pode citar ou adaptar o texto upstream conforme a licença de cada um — CC BY 4.0 exige manter TASL, a nota de alterações (Change Notice) e o aviso de isenção de responsabilidade na mesma visualização; Apache-2.0 exige manter LICENSE/NOTICE. A introdução no site traz apenas resumos e links; o texto adaptado e a atribuição por documento ficam no repositório, em [`docs/plugin-dev/standards/` (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) e [`docs/dev/standards.md` (disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md).

| Camada | Pergunta que responde | Quem usa | Onde fica no sistema |
| --- | --- | --- | --- |
| ENTRY | Se a forma mínima de um plugin é válida (entrada, manifesto, licença) | Autor, CI do repositório do autor | Alinhada à verificação de forma de pacote feita na pré-verificação de publicação |
| FLOW | Se o fluxo do plugin no uso real se sustenta (instalação, ativação, desinstalação, recuperação de falhas) | Autor, testes | A introdução dos exemplos explica como reproduzir |
| BOOST | Se experiência e componibilidade estão no nível esperado (comandos, renderização, skills, múltiplas plataformas) | Autor, revisores | Os exemplos são apresentados por superfície envolvida |

## Como usar ENTRY / FLOW / BOOST no lado da empresa

| Uso | Como fazer | O que não fazer |
| --- | --- | --- |
| Comprovação do autor | Rodar o checker upstream no CI do repositório do autor (`dsh-standards@1.6.9`, exige Node ≥24.9) e fixar comando e versão | Não trate o resultado da verificação upstream como conclusão de admissão da empresa |
| Admissão da empresa | Após o envio, ver o detalhe da pré-verificação do próprio dsh-ee (pass / warn / fail e conflitos de publicação) | Não use a verificação upstream no lugar da pré-verificação |
| Apoio à revisão | Na revisão, perguntar com base nos exemplos e nas cláusulas das normas | Não classifique nem ordene plugins ou exemplos na página |
| Percurso de aprendizado | Ler o código dos exemplos do nível iniciante ao avançado | Não trate os exemplos como recomendação oficial |

## O que o dsh-ee não faz

- Não executa checkers de terceiros no servidor nem oferece um sandbox para rodar verificações.
- Não converte resultados de verificação upstream em níveis de qualidade, notas ou ordenação.
- Não trata o texto adaptado das normas como texto upstream: citação ou adaptação deve manter a origem, a licença (CC BY 4.0 / Apache-2.0) e o Change Notice, com atribuição por documento adaptado.
- Não promete «recomendação oficial»: os exemplos apenas mostram que tipo de capacidade eles apresentam.

## Como o autor deixa evidências

1. Rode na máquina com `dsh-standards@1.6.9` fixo (Node ≥24.9) e escreva o comando no CI do repositório do autor.
2. Salve o relatório JSON ou HTML como artefato de build, registrando a data de geração e a versão do host.
3. Na nota de publicação, cite onde está o relatório em vez de escrever apenas uma conclusão como «já verificado».
4. Quando as normas upstream forem atualizadas, rode de novo; marque os relatórios antigos com a versão e não os misture.

```bash
# 固定版本，避免 npx 同名包冲突
npx dsh-standards@1.6.9 check .
npx dsh-standards@1.6.9 check . --json > report.json
```

O código de saída do checker, os campos do relatório e a forma de ler «not-run / NA / static» (que não significam aprovação) seguem a referência do autor no repositório (ver os links abaixo).

## Relação com a página de exemplos

A página de exemplos mostra «um caminho mínimo completo e executável»: cada exemplo explica o que se aprende, os pré-requisitos, a versão fixada e o comando de instalação, e a página de detalhes informa origem e licença. Os exemplos não trazem conclusão de verificação, e a página não ordena exemplos pelo resultado das verificações.

## Textos completos no repositório

Esta página do site é apenas uma introdução; o texto das normas, o índice de cláusulas e o uso do checker estão na documentação do repositório e no repositório upstream (os links apontam para os documentos após o merge):

- [Normas e verificações (docs/dev/standards.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md)
- [Referência para autores de normas e checker (docs/plugin-dev/standards/README.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md)
- [Comandos do checker e campos do relatório (docs/plugin-dev/standards/checker-cli.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/checker-cli.md)
- [Limites da evidência e leituras equivocadas comuns (docs/plugin-dev/standards/evidence-and-misreadings.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/evidence-and-misreadings.md)
- [Mapeamento entre normas e critérios de admissão (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Upstream: repositório dsh-standards](https://github.com/PerryLink/dsh-standards) · [npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)
- [Upstream: dsh-primer (guia e toolchain de desenvolvimento de plugins DSH)](https://github.com/PerryLink/dsh-primer)
