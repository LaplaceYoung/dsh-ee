# 扩展

dsh-ee 的扩展把企业自己的系统接进来（设计见 [docs/plugins.md](../docs/plugins.md)）。SDK、示例、规格和技能都在这个目录下，和服务端一起改、一起测。

| 目录 | 内容 |
|---|---|
| [examples/demo-declarative](./examples/demo-declarative) | 声明式扩展的示例：清单 + OpenAPI 文档，不写代码。复制它就能开始 |
| [official/ee-jwt-adapter](./official/ee-jwt-adapter) | 企业系统接收 `ee_jwt` 的适配层样例：从 dsh-ee JWKS 取公钥、验签，再映射成自己的会话 |
| [schemas](./schemas) | 机器能读的规格：清单的 JSON Schema |
| [skills/dsh-ee-extension-dev](./skills/dsh-ee-extension-dev/SKILL.md) | 给 Agent 的技能：照着做就能写出一个通过检查的扩展 |

写完用 `dsh-ee extension check <目录>` 检查，`--json` 给程序和 Agent 看。规格用 `dsh-ee schema list` / `dsh-ee schema get <名字>` 查，离线可用。

管理端「企业连接 → 安装声明式扩展」已支持上传声明式 ZIP、预检、填写凭据与固定配置、安装并运行对应的 OpenAPI 连接；范围见 [declarative-install.md](../docs/declarative-install.md)。服务扩展、SDK、应用级参数绑定、多份共享凭据和飞书 D1 在后面的批次（docs/plugins.md §12）。
