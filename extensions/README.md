# 扩展

dsh-ee 的扩展把企业自己的系统接进来（设计见 [docs/plugins.md](../docs/plugins.md)）。SDK、示例、规格和技能都在这个目录下，和服务端一起改、一起测。

| 目录 | 内容 |
|---|---|
| [examples/demo-declarative](./examples/demo-declarative) | 声明式扩展的示例：清单 + OpenAPI 文档，不写代码。复制它就能开始 |
| [schemas](./schemas) | 机器能读的规格：清单的 JSON Schema |
| [skills/dsh-ee-extension-dev](./skills/dsh-ee-extension-dev/SKILL.md) | 给 Agent 的技能：照着做就能写出一个通过检查的扩展 |

写完用 `dsh-ee extension check <目录>` 检查，`--json` 给程序和 Agent 看。规格用 `dsh-ee schema list` / `dsh-ee schema get <名字>` 查，离线可用。

检查通过后，在管理后台「企业连接 → 安装扩展」里选扩展的 zip 包（或同时选中清单和接口文档），看过安装计划（请求会发往哪些地址、用谁的身份、哪些是写操作、要设置的参数）再装成企业连接。装过的同一个扩展（按 `id`）再装就是升级。

服务扩展、端侧扩展和 SDK 在后面的批次（docs/plugins.md §12）。
