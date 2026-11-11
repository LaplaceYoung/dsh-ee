# 规格文件

扩展相关的约定都发布成机器能读的规格，Agent 和命令行用的是同一份（[docs/agent-dev.md](../../docs/agent-dev.md) §3）。

| 规格 | 文件 | 命令 |
|---|---|---|
| 清单 `dsh-ee.extension.json` | [manifest.schema.json](./manifest.schema.json) | `dsh-ee schema get manifest` |
| `dsh-ee extension check` 的检查项 | 由代码生成 | `dsh-ee schema get errors` |

这里的文件和编译进二进制的那一份（`server/internal/extension/schemas/`）内容相同，有测试保证；改的时候两处一起改。

## 路径写法

清单里的 `openapi`、`apps[]`、`icon` 可以写 `openapi.yaml`，也可以写 `./openapi.yaml`；两者等价。绝对路径、`..`、反斜杠仍会被拒绝。`dsh-ee extension check` 和网页安装用的是同一份校验。
