# dsh-ee 设计稿（v0.1，2026-10-26）

## 定位

> 2026-11-11 更新：定位已从最初的「分发中心」扩展为企业能力中心/治理平台（插件索引、npm registry、审核、知识库、工作台、模型网关、DSH 分发）。本稿保留当时的设计依据；当前范围以 README 与 roadmap 为准。

企业私有部署的 **DSH 能力中心**：插件索引源 + npm registry + 审核流程 + 知识库/工作台 + 模型网关 + DSH 本体分发。
不是私有化的 dshfind 网站——论坛、课程、排行、SEO 不搬。

## 设计依据（来自 dshfind 的数据与上游契约）

| 事实 | 影响 |
|---|---|
| `dsh plugin add` 在 profile 目录内转发给 pnpm | 私有市场的核心是 npm registry，索引只是目录 |
| 索引 format 1 的版本来源只有 `npm` / `git`（40 位 SHA） | 客户端要同时配「索引源 + registry」 |
| 读端任何一条违规就整份拒收；不跟随重定向 | 复用 dshfind `scripts/lib/dsh-index.mjs` 的自检；索引直接 200 |
| 1.9 万仓库中 npm 8567 / git 8145 / release 941 / 需构建 542 | 约一半插件未发 npm，「代构建」是后续差异化点 |
| git 安装需 allowBuilds，代码在安装时执行、不在沙箱内 | 安全审核是刚需 |
| 国内 npmmirror 下载量约为 npm 的 43% | 在线上游支持 npmjs / npmmirror / 自定义 |

## 模块

1. **一键部署**：docker compose + 离线安装包（镜像 tar + install.sh）；默认 SQLite + 本地磁盘，可选 Postgres + S3/MinIO。安装向导生成企业 CA 相关的客户端配置。
2. **客户端一键接入**：`curl …/setup.sh | sh` / PowerShell，写入索引源、`~/.npmrc`（registry + 个人 token + cafile）、`NODE_EXTRA_CA_CERTS`。
3. **内置 registry**：自实现 npm 协议子集（packument / tarball / publish / dist-tags / token）；审核状态、可见范围、下架直接做进 registry。企业作用域（如 `@acme`）**永不回源**，防依赖混淆。
4. **上游预载**
   - 在线：订阅 dshfind 元数据 feed，按 Top N / 分类 / 评分门槛 / 离线可装 挑选，连同依赖闭包与企业推荐 DSH 版本对应的 `@deepseek-ai/dsh-*` 核心包一起同步；新版本策略：自动 / 同步后审核 / 不同步。
   - 离线：dshfind 每月发布 ed25519 签名的种子包（`.tar.zst`）；DMZ 实例可导出增量包摆渡。导入时校验签名与逐包 integrity。
   - 识别 postinstall 下载二进制的依赖，标「需外网」。
5. **内部分享**：`pnpm publish` 或网页上传 `.tgz`；强制企业作用域；自动检查（dsh.bundle、安装脚本、漏洞、License、密钥泄露）→ 审批 → 上架；可见范围：全公司 / 部门 / 仅自己。员工也可「申请引入」社区插件。
6. **治理**：白名单（默认）/ 开放 + 黑名单；一键下架；registry 下载日志即安装审计与内部热门榜。
7. **DSH 本体分发（新增）**
   - 分发 DSH 核心（CLI）与桌面版（官方 + 可选的社区桌面端），各平台各架构安装包。
   - 安装包从官方渠道同步，核对 sha256 后才入内网；门户展示 sha256 与校验命令。
   - 管理员设「企业推荐版本」：门户默认展示它，同时作为插件兼容性检查和 dsh-* 核心包镜像的基准。
   - 可控：是否开启分发、是否向员工展示预览版、是否分发社区桌面端。
   - 离线增量包可选择包含安装包。
   - 规划：企业预配置安装包（内置索引源 / registry / CA），需确认 DSH 是否支持预置配置。
8. **评分可开关（新增）**
   - 总开关「启用评分」；子项：显示 dshfind 上游评分、允许员工打分（1-5 星，仅企业内可见）、预载默认最低评分。
   - 关闭后：门户与后台不显示任何评分，排序选项中移除评分项，预载挑选不能按评分排序或设门槛（已选中的评分排序自动退回默认）。
9. **认证**：网页 本地账号 / LDAP / OIDC；pnpm 用个人 token。

## 优先级

- **P0**：部署、内置 registry、索引输出、在线预载 + 离线导入、内部发布 + 单级审批、客户端接入脚本、本地账号 + LDAP、DSH 本体分发、评分开关。
- **P1**：OIDC、漏洞 / License 扫描、部门可见、安装看板、对接已有 Nexus / Artifactory。
- **P2**：git / release 插件代构建、多 DSH 版本兼容矩阵、企业预配置安装包。
- **不做**：论坛、课程、积分排行、多租户 SaaS、插件运行时沙箱。

## 待在上游 deepseek-harness 核实

1. `npm` 来源能否指定 registry，还是只依赖 `.npmrc`？`dsh plugin add` 是否读取 profile / 用户级 `.npmrc`？
2. 索引源请求能否带 Authorization？（决定部门可见能否做到索引层）
3. 多个索引源中出现同名包时的优先级。
4. 是否有可集中下发的配置（禁用公网源、预置索引源）？否则只能靠网络层屏蔽公网 registry。
5. DSH 核心与桌面版的官方发布渠道、包名与安装包格式（原型里是占位）。
