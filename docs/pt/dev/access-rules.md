# Permissões e regras de acesso: referência rápida

## Três camadas

O controle de acesso do dsh-ee é dividido em três camadas; os pontos de alteração ficam no painel administrativo e entram em vigor imediatamente após a configuração.

| Camada | Pergunta que responde | Ponto de configuração | Quem é afetado após configurar |
| --- | --- | --- | --- |
| Pontos de permissão | Se esta pessoa pode usar este recurso | «Perfis e permissões» | Exibição/ocultação de páginas, chamadas de API |
| Campos e regras de acesso | Se este campo e este dado podem ser vistos ou alterados | «Campos e regras de acesso» | Campos de extensão de documentos, bases de conhecimento, usuários etc. |
| Restrições administrativas | Se alterações em configurações críticas exigem aprovação | «Campos e regras de acesso» | Operações de alteração de valores de campos e de configurações |

## Como os pontos de permissão chegam às páginas e às APIs

O menu só decide «se aparece»; o limite real está na API. Esconder botões na página não constitui limite de segurança.

| Ponto de permissão comum | Função | Funcionário tem por padrão |
| --- | --- | --- |
| `portal.view` | Usar a Área de trabalho, os aplicativos da empresa, a documentação de desenvolvimento e outras páginas de funcionário | Sim |
| `registry.publish` | Publicar plugins, skills, prompts e aplicativos | Sim |
| `extensions.develop` | Receber a skill e as instruções de comando de desenvolvimento de extensões | Não; precisa ser concedido pelo administrador |
| `plugins.manage` | Revisar, publicar e despublicar capacidades | Não |
| `connections.manage` | Integrar e configurar conexões da empresa | Não |

Os pontos de permissão são sincronizados do catálogo interno do código para o banco de dados; os pontos novos de uma versão são concedidos automaticamente ao perfil «Funcionário» (se o catálogo indicar um perfil padrão).

## Ordem de avaliação das regras de acesso

1. Superadministrador: não está sujeito às regras (a menos que o superadministrador esteja desativado no sistema).
2. Permissões de perfil: primeiro decide se o recurso em si está visível.
3. Regras de acesso a campos: avalia read / write / deny campo a campo; as regras podem ter condições.
4. Restrições administrativas: se a operação de escrita exige enviar a alteração antes e quem a aprova.

Depois que uma camada anterior chega a uma conclusão, as camadas seguintes não a flexibilizam.

## Pontos de configuração e exemplos

| Quero | Onde ir |
| --- | --- |
| Liberar material de desenvolvimento de extensões para um tipo de funcionário | «Perfis e permissões» → conceder `extensions.develop` |
| Fazer um campo ser visível só para o próprio departamento | «Campos e regras de acesso» → criar uma regra read com condição de departamento |
| Proibir a exportação do campo «confidencial» dos documentos | O mesmo local: adicionar uma regra deny e conferir o retorno da API |
| Exigir aprovação para alterar campos críticos | «Campos e regras de acesso» → Restrições administrativas → vincular uma solicitação de mudança |

## Como cooperar ao escrever extensões

- Declare no manifesto os pontos de permissão e os nomes de credencial necessários, sem colocar a credencial no código ou no manifesto.
- As regras de leitura e escrita dos campos são configuradas pelo administrador; a extensão trata apenas os campos visíveis retornados pela API e não adivinha permissões.
- Diferencie nos códigos de erro «sem permissão», «não encontrado» e «parâmetro inválido»; não retorne tudo como 500.

## Textos completos no repositório

Esta página do site é apenas uma referência rápida; os detalhes das regras, a aprovação de instalação de extensões e o catálogo de pontos de permissão ficam na documentação do repositório (os links apontam para os documentos após o merge):

- [Design do controle de acesso (docs/access-control.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/access-control.md)
- [Publicação, revisão e permissões da empresa (docs/enterprise-publishing.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Visão geral do protocolo de extensões v1 · credenciais e operações de escrita (docs/extension-protocol/README.md, disponível após o merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [Texto do protocolo de extensões (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [Skill de configuração de permissões (extensions/skills/dsh-ee-access-config/SKILL.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/skills/dsh-ee-access-config/SKILL.md)
