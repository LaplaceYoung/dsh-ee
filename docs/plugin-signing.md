# 插件/扩展签名、企业目录、自助分发与回滚：设计选项（部分实现）

> **状态：部分实现（#134 扩展签名 + 企业目录 v1，2026-11-13）。** 扩展签名做了最小闭环，并把信任根落了库：`dsh-ee extension keygen / sign / verify`、安装计划显示 `unsigned / valid / invalid / revoked`、`trusted_signing_keys` 表 + `/api/admin/signing-keys` 管理接口（换下 / 撤销保留历史）、`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 强制验签；`DSH_EE_EXTENSION_TRUSTED_KEYS` 仍作为只读兼容来源。企业目录 v1 做了「format 2 文档 + 目录整体 Ed25519 签名 + check/apply/list + `directory_entries` 登记」；登记后由管理员显式触发条目安装 / 升级（`POST /api/admin/directory/entries/{id}/install|upgrade`：扩展按登记的固定 tarball 下载后复用安装计划，插件只认 npm 固定版本、走现有预载 + 审核），不做自动安装 / 自动升级，见 §8。 企业目录 v1.1 加了**只读**的更新检查 `GET /api/admin/directory/updates`（登记条目 vs 已装扩展连接 / 企业源已上架插件版本，返回 `available / current / not_installed / unknown`）和 format 2 的可选 `auto_upgrade` 声明 + 设置 `directory.autoUpgrade`（默认 false）；`auto_upgrade` 本期只登记 / 校验 / 展示，**不执行自动升级**，见 §8。
>
> 管理页面在管理后台「能力与连接 → 扩展签名」（列表 / 新增 PEM 公钥 / 换下 / 撤销、只读环境变量来源）；企业目录的入口是「能力与连接 → 企业插件目录」（导入 / 校验 / 登记目录 JSON，对登记过的条目看计划再安装 / 升级）。
>
> **还没做**：从目录 URL 自动拉取 / 缓存 / 离线包导入、作者自助上传扩展、扩展版本回滚、DSH 插件（npm 包）签名。下面「如果要做」的选项、取舍和风险仍然成立，未实现的部分供维护者决定；已实现部分的口径、接口和边界见 §7、§8。
>
> 设计草案的原始出处是 [plugins.md](./plugins.md) §9.2–§9.5（企业目录、签名、升级、卸载），本文只做聚焦整理并补上现状证据与风险。相关：`docs/extension-authors.md` §5.3（升级与回滚边界，见 #130）、`docs/enterprise-publishing.md` §8（当前未实现清单）、`docs/cross-site-embedding.md`（另一条实现参考：跨站嵌入，见 #130）。

## 1. 现状（代码证据）

### 1.1 扩展（声明式 / 服务）

| 环节 | 现在怎么工作 | 证据 |
|---|---|---|
| 交付 | 作者用 `dsh-ee extension check/pack/report` 自检并把 zip（或服务地址）交给管理员；**没有作者自助上传通道** | `server/cmd/dsh-ee/cli_ext.go`、`server/internal/api/routes.go`（只有 admin 的 check/install） |
| 检查 | `extension check` 是**静态检查**：清单、接口文档、只读标记、凭据写法，不执行扩展代码 | `server/internal/extension/check.go`、`codes.go`（62 个检查项） |
| 身份 | 扩展身份 = `id@version` + **sha256 摘要**（`extension.Digest`，对清单与 OpenAPI 字节算）；签了名时还有 `signature.json` | `server/internal/extension/manifest.go`、`signature.go` |
| 摘要的用途 | 用于判断「同版本内容是否变了」、安装计划里展示、升级时决定要不要管理员确认；**摘要不是作者身份认证**，「是谁签的、内容有没有被换」由签名回答（#134） | `server/internal/api/extensions.go` 的 `planExtension` |
| 安装 | 管理员上传 zip / 填服务地址 → `POST /api/admin/extensions/check` 出安装计划 → `POST /api/admin/extensions/install`；升级时敏感变化（新地址、新增反向权限、同版本内容变化、凭据模式变化）必须带 `confirm:true` | `extensions.go`、`routes.go` |
| 签名 | 已实现最小闭环（#134）：作者用 `dsh-ee extension sign` 对清单 + 接口文档签 `signature.json`（Ed25519，签 `作者/名字@版本:摘要`）；安装 / 检查计划显示 `unsigned / valid / invalid / revoked`。默认只提示不阻断，`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` fail closed | `server/internal/extension/signature.go`、`server/cmd/dsh-ee/cli_sign.go`、`server/internal/api/extensions.go` |
| 信任根 | 受信任公钥落库（`trusted_signing_keys`：key id / 公钥 / purpose / valid_from / retired_at / revoked_at / created_by / note）；换下、撤销保留历史，撤销后新装 / 升级拒绝。`DSH_EE_EXTENSION_TRUSTED_KEYS` 仍是只读兼容来源 | `server/internal/store/migrations/0032_trusted_signing_keys.sql`、`server/internal/api/signing_keys.go`、`store/signing_keys.go` |
| 回滚 | **无**：升级就是再装一次；只记录 `upgraded_from`，没有「回到上一版本」的接口或页面；声明式扩展的旧版本 spec 仍存在 `asset_versions` 表里，但只是历史数据，没有回滚入口 | `extensions.go:652`、`store/assets.go`、`store/migrations/0007_hub.sql` |
| 企业目录 | v1 已实现「管理员贴入整体签过名的 `dsh-ee-directory.json` → 验签 + 逐条校验 → 登记 `directory_entries`」；登记后可对单条显式 `install` / `upgrade`：扩展按登记的固定 tarball 地址下载、核对身份/摘要后复用安装计划，插件只认 npm 固定版本并走现有预载 + 审核。不自动安装、不自动升级，也不从 URL 自动拉取目录；v1.1 加了只读更新检查（`GET /api/admin/directory/updates`，登记条目 vs 扩展连接 / 企业源已上架版本）和条目可选的 `auto_upgrade` 声明 / 设置 `directory.autoUpgrade`（只登记 / 校验 / 展示，不执行）。管理页在「能力与连接 → 企业插件目录」 | `server/internal/directory/`、`server/internal/api/directory.go`、`server/internal/api/directory_install.go`、`server/internal/api/directory_updates.go`、`web/src/pages/admin/directory.tsx`、`store/migrations/0034_directory_entries.sql`、`store/migrations/0035_directory_auto_upgrade.sql` |

### 1.2 DSH 插件（npm 包）

| 环节 | 现在怎么工作 | 证据 |
|---|---|---|
| 发布 | 作者 `npm publish` 到企业源（需要账号「发布能力」+ 令牌 `registry:publish`）或网页上传 `.tgz`；默认进审核 | `server/internal/api/npm.go`、`server/README.md`「npm 源」 |
| 完整性 | 发布时按 tarball 计算 sha512/shasum；声明了 `dist.integrity`/`shasum` 的会校验；镜像、离线包导入也重算 | `internal/registry/service.go`、`internal/registry/bundle.go` |
| npm 签名 | 上游 packument 里与地址无关的 `dist.signatures` 会在 integrity 一致时原样保留，供 corepack 等客户端验证上游签名（如 pnpm 自身）；**dsh-ee 自己不验签，也不给企业内发布的包签** | `internal/registry/packument.go` 注释与代码 |
| 审核 | 逐版本 `pending → published/rejected`，可 `unpublished`/`restore`；同版本号不能重发 | `internal/registry/service.go` 的 `Review`、`docs/enterprise-publishing.md` §3 |
| 回滚 | 下架/恢复是**状态回退**；dist-tag 可以指回旧版本，但已装客户端不会自动降级，dsh-ee 也没有「回滚到上一版本」的入口 | 同上 |

### 1.3 相关现状（容易混淆，别当成已实现）

- 配置变更（角色/字段/访问规则）有**回滚**接口 `POST /api/admin/config/changes/{id}/rollback`，与扩展/插件回滚无关。
- 离线包导入是**内容完整性**校验（sha512 对文件名/清单），不是签名校验。
- 服务扩展的信任全在管理员：安装时把服务地址、共享凭据、`ee_jwt` 身份交给扩展；服务端不做代码签名（代码在企业自己手里）。

## 2. 设计选项

> 下面每个选项都标「若做，建议的最小形态」与「风险」。**2.1 A 的「签清单 + 接口文档」「Ed25519 detached 签名」「公钥信任库」「安装计划展示」「默认警告后允许」已按建议实现最小闭环（§7）**；**2.2 B 的企业目录 v1（贴入 JSON、验签、逐条校验、登记）已实现（§8），但 URL 拉取/缓存/离线导入仍未实现**；其余选项（zip 整包签名、npm 包签名、自助分发、回滚）仍未实现，维护者确认后应单独立项/PR。

### 2.1 A. 签名（扩展优先，插件其次）

**目标**：让管理员在安装前能判断「这份清单/接口文档/包确实由某个可识别的主体发布，且内容没被替换」。

| 设计点 | 选项 | 建议（若做） |
|---|---|---|
| 签什么 | ① 清单 + 接口文档（= 现有 `Digest` 的输入）；② zip 整包；③ 服务扩展的实时清单 | ① 最小可用：签摘要，和现有身份/变更检测同一口径；② 覆盖「zip 里的其他文件」，但声明式扩展只认清单+OpenAPI，收益有限；③ 服务扩展代码不在 dsh-ee 手里，签名只能签「清单声明」 |
| 算法与格式 | ① detached 签名文件（如 `dsh-ee.extension.sig`，JSON：`{version, key_id, alg, signature}`）；② 塞进清单字段；③ 复用 JWT | ① 独立文件，不改变清单 v1 结构、老工具忽略即可；签名对象建议为 `"<id>@<version>:<digest>"` 做域分隔（与 npm `dist.signatures` 的「名字@版本:integrity」思路一致），避免签名被挪用到别的身份 |
| 算法 | Ed25519（Go 标准库，短签名）或 RSA-PSS/RS256（复用现有 `eejwt` 的 RSA 密钥管理） | Ed25519；如需和 `ee_jwt` 共用密钥体系再评估 RSA |
| 密钥管理 | ① 作者本地私钥，dsh-ee 只存公钥；② dsh-ee 生成/托管私钥（官方扩展）；③ 企业 CA / KMS 托管 | ①+②：公开/官方扩展用 dsh-ee 官方密钥签名；企业自己签的扩展由企业管理员导入公钥；**私钥不进 dsh-ee 数据库**（避免主密钥泄露时连签名私钥一起丢） |
| 验签时机 | ① 只在安装/升级时验；② 每次启动/健康检查也验；③ 下载/分发时验 | ① 必须；② 服务扩展的清单会变，建议健康检查发现 `digest` 变化时要求重新确认（现有行为）+ 验签；③ 企业目录/自助分发时再说 |
| 策略 | 允许 / 警告后允许（默认）/ 拒绝；受信任公钥列表（官方/企业/供应商） | 对齐 [plugins.md](./plugins.md) §9.3 的「默认警告后允许」：dsh-ee 没有官方市场，强制签名会把第三方都挡在外面；企业可以切到「拒绝」 |
| 展示 | 安装计划与审核页显示：签名状态、签名者、key id、是否受信任、内容摘要 | 区分「已签名但不信任」「未签名」「签名无效」三种，不要合并成一个绿勾 |
| 签名不覆盖什么 | 服务扩展运行时行为、端侧扩展本机执行、凭据是否会外送、安装脚本内容 | 文档与 UI 必须写明：签名 ≠ 安全审计，仍需人工/静态检查（[plugin-quality.md](./plugin-quality.md)） |

**插件（npm 包）签名的最小形态**：在企业内发布时由作者/企业 CI 对 tarball 的 `integrity` 签名，dsh-ee 在 `npm_versions` 记录签名与 key id，审核页展示；也可以在 packument 的 `dist.signatures` 里给内部包生成一条（格式与 npm 生态兼容），但需要先确认客户端是否会用、以及 key 轮换/吊销语义。**不建议**只把签名放在包里（包内签名可自证但不可信来源）。

**风险**：

- 签名只证明「谁签了这段内容」，不证明扩展安全；容易被误读成「已认证」；
- 密钥丢失/轮换/吊销、多环境（生产/测试）密钥混用；
- 签名对象规范化（换行/编码/字段顺序）不一致会导致「内容没变但验签失败」；
- 企业没有密钥基础设施时，签名会变成走过场或全员用同一把密钥；
- 服务扩展的实际行为由企业部署决定，签清单不等于签运行中的代码。

### 2.2 B. 企业扩展目录（v1 已实现「登记」，见 §8）

**目标**：企业维护一份「审核过的扩展清单」（HTML/JSON 目录），管理员从目录安装，而不是每次手填地址/上传 zip。

> 已实现的部分：format 2 目录文档 + 目录整体 Ed25519 签名 + 管理员贴入 JSON 的 `check / apply / list` + `directory_entries` 登记；tarball 地址强制 https、默认只接受公网（可用 `DSH_EE_DIRECTORY_TARBALL_HOSTS` 白名单放行内网对象存储）。登记后由管理员逐条触发 `install` / `upgrade`：扩展从登记的固定 tarball 下载、核对身份/摘要后复用安装计划（含 confirm / 敏感项 / 签名策略，条目声明了签名者的还要求包确实由该钥匙签名）；插件只认 npm 固定包名 + 固定版本，走现有预载 + 审核，不绕过审核。**不自动安装、不自动升级**；v1.1 提供只读的更新检查（对比扩展连接 / 企业源已上架版本，见 §8），并允许条目声明 `auto_upgrade`（本期只登记 / 展示，不执行）；从 URL 自动拉取目录、缓存、ETag、离线包导入仍未实现。

| 设计点 | 选项 | 建议（若做） |
|---|---|---|
| 目录格式 | ① JSON（扩展 id/version/source/digest/signature/审核人）; ② 复用 `dsh-ee-bundle.json` 的离线包清单格式 | ① 独立 JSON（字段与安装计划对齐）；② 离线包已经有完整性/导入流程，但目标是「在线目录」，不要混 |
| 目录托管 | ① 企业内 Git/静态站（HTTPS）; ② dsh-ee 自己生成并提供目录端点 | ① 先支持「从 URL 拉取 + 缓存 + 手工刷新」；② 需要先有签名和发布流程，成本更高 |
| 信任 | 目录本身要不要签名？目录里的条目要不要必须带签名？ | 目录是管理员配置的可信来源（HTTPS + 可选签名）；条目按 A 的签名策略校验；**目录不替代审核** |
| 安装流程 | 目录浏览 → 选版本 → 看安装计划（身份/签名/来源/权限）→ 确认安装 | 复用现有 check/install；目录只负责「来源」。 |
| 更新 | 目录刷新频率、离线缓存、不可达时行为 | 缓存上次成功结果；不可达时不让新装，但不影响已装扩展 |
| 撤销 | 目录里删条目/标 revoked 时，已装扩展怎么办 | 只做「提示/告警」，不自动卸载；自动卸载风险高 |

**风险**：目录是新的信任根（被篡改 = 批量装错）；维护成本（谁审、多久更新）；目录与 dsh-ee 版本兼容性；离线/内网环境拉不到；条目字段漂移。

### 2.3 C. 自助分发（作者上传/发布扩展）

**目标**：扩展作者不经过管理员手动转交，自己能提交、更新、查看状态。

| 设计点 | 选项 | 建议（若做） |
|---|---|---|
| 上传入口 | ① 作者在网页上传 zip（新权限点，如 `extensions.develop`）; ② 用 npm 包承载扩展（复用 registry 发布/审核）; ③ Git 仓库地址 | ② 最省事：扩展已经是一个目录 + 清单，可以打成 npm 包走现有发布/审核/权限；但要处理「声明式扩展 vs 服务扩展」不同安装语义 |
| 审核 | 必须保留管理员审核（现在就是管理员安装）；自动检查（62 项）先跑，失败不阻塞提交但阻塞自动通过 | 与 DSH 插件一致：提交 → `pending` → 管理员通过 → 可安装 |
| 隔离 | 上传的文件先放隔离区，扫描/解包限量（现有 `MaxPackage` 8 MiB、`maxFiles` 64） | 沿用现有上限 + 每用户配额（设计） |
| 签名 | 可选：企业策略「只接受已签名扩展」时，上传必须带签名 | 依赖 A |
| 服务扩展 | 上传的只是清单/接口文档/说明，实际运行在企业自己的服务里 | 安装仍要管理员填地址/确认凭据与权限 |
| 撤销 | 作者下架、管理员强制下架、已装用户感知 | 只对未装/升级生效；已装版本保留（避免远程「撤回」破坏在跑的系统） |

**风险**：垃圾/重复提交、存储与扫描成本；作者身份与权限（谁能代表企业发布）；上传包可作为供应链入口（签名/审核缺一不可）；服务扩展「上传清单」与实际部署不一致；审核人力。

### 2.4 D. 版本回滚

**目标**：扩展升级后出问题，能回到上一个已审过的版本，而不是「再装一次旧包」。

| 设计点 | 选项 | 建议（若做） |
|---|---|---|
| 回滚对象 | ① 扩展（含它生成的连接配置/工具开关）; ② DSH 插件（npm 版本）; ③ 两者都做 | ① 先做扩展：数据层已有 `asset_versions` 历史；② npm 版本不可重发，回滚语义应该是「pin 到旧版本 + 重新审核」，成本不同 |
| 实现方式 | ① 把旧版本 spec **复制成新版本**（`asset_versions` 追加，标记 rollback_of）; ② 直接把 live 指针切回旧版本 | ① 可审计、可再次审阅、不影响历史；② 会绕过「升级要确认」的审计链，不建议 |
| 权限 | 需要 `connections.manage` + 二次确认；是否要求重新审核 | 至少管理员确认；企业策略可以要求重新审核（与安装同级） |
| 签名 | 回滚版本也必须是「已审过/已签名」的版本；不能借回滚引入未审内容 | 回滚时重新执行 check + 验签 |
| 数据 | 扩展配置、共享凭据、字段、员工授权是否跟着回滚 | **不自动回滚数据**：配置/凭据是资源，不是版本内容；回滚只换 spec，缺的配置在安装计划里提示 |
| 服务扩展 | 外部服务代码不在 dsh-ee 手里，回滚清单不等于回滚对方服务 | UI 明示；只回滚「声明与工具开关」 |
| 回滚链 | 支持回滚到任意历史版本还是只上一版 | 先只上一版（`upgraded_from` 已有），多个历史版本需要 UI/保留策略 |
| 与「审计」的关系 | 回滚也要写审计（谁、从哪个版本到哪个版本、原因） | 必须 |

**风险**：旧版本有已知漏洞/已撤销权限，回滚会把问题带回来（必须保留审核/签名）；配置不配套导致「回滚后连不上」；服务扩展/端侧扩展的版本由外部决定；npm 插件「回滚」在现有协议里没有定义（下架/恢复 ≠ 降级）；保留策略与存储成本。

## 3. 共同问题：信任根与生命周期

| 问题 | 说明 | 建议 |
|---|---|---|
| 信任什么 | 作者身份？内容摘要？审核记录？部署的服务？ | 三者分开表达：签名（内容来源）、审核（企业准入）、部署（运行时）；UI/文档不要合并成「可信」一个词 |
| 密钥轮换/吊销 | 签名公钥的启用/停用、历史签名怎么办 | 公钥带 `kid` + `not_before/not_after`；吊销后仍允许验证历史版本（标注「已吊销」），但拒绝新装。**已按此实现（#134）**：`key_id` + `valid_from` + `retired_at/revoked_at`，revoked 显示且拒绝新装，见 §7 |
| 多实例 | 企业目录缓存、密钥列表、回滚状态要共享 | 都要落库；内存缓存可接受但要有失效策略 |
| 离线/内网 | 目录、签名、密钥都在内网 | 目录与公钥支持手工导入/离线包；不依赖公网 |
| 供应链边界 | 上游 npm 包的安全由作者/上游生态负责；dsh-ee 不做代码审计 | 文档必须写清（见 `plugin-quality.md` §1、`plugin-audit` 类边界） |
| 对外措辞 | 不能把「有摘要/有审核」说成「已签名/已认证/安全审计通过」 | 至少在审核页、作者指南、README 统一措辞 |

## 4. 如果维护者决定要做：建议顺序

1. **签名（扩展）**：✅ 已做（#134）：`extension keygen/sign/verify` + 公钥信任库（落库）+ 安装计划展示 + 策略（默认警告后允许，`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 切到拒绝）。没动安装主流程。
2. **回滚（扩展）**：基于 `asset_versions` 追加新版本 + 审计 + 二次确认；不自动回滚配置/凭据。
3. **企业目录**：✅ v1 已做（§8）「管理员贴入签过名的清单 → 验签 + 逐条校验 + 登记」，v1.1 加了只读更新检查与 `auto_upgrade` 声明（只登记 / 不执行）；「从 URL 拉取 + 缓存 + 离线包」仍未做。
4. **自助分发**：最后做，先复用 DSH 插件的 npm 发布/审核通道，再评估扩展 zip 上传；需要新的权限点和配额。

顺序理由：签名是目录和自助分发的前提；回滚相对独立、收益直接；自助分发引入新的对外接口和滥用面，放到最后。每步都要有：数据迁移（只增不改）、默认关闭/可回滚、审计、文档与「未实现」状态同步。

## 5. 明确不做（当前）

- 不生成/托管作者私钥、不声称任何包「已认证」；扩展签名已实现最小闭环（§7），DSH 插件（npm 包）签名仍未实现；
- 不实现从 URL 自动拉取目录 / 目录缓存 / 离线包导入（v1 只接受管理员贴入并整体签过名的 JSON，见 §8）、Git/GitLab 安装；**不执行自动升级**：目录条目的 `auto_upgrade` 和设置 `directory.autoUpgrade` 本期只登记 / 校验 / 展示，没有任何自动安装 / 升级动作；
- 不实现作者自助上传扩展、扩展市场/评分/排名；
- 不实现扩展一键回滚；
- 不给 npm 协议加 dsh-ee 自己的签名/降级语义；
- 不在没有安全评审的情况下把上述能力做成默认开启。

## 6. 待维护者决策

1. 签名的优先级：扩展已做最小闭环（§7）；DSH 插件（npm 包）签名是否也做仍待定。
2. 签名策略默认值：已实现为「警告后允许」默认、`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 切到拒绝（fail closed）。
3. 官方/企业/供应商公钥的信任列表由谁维护、放在哪：现在在库表 `trusted_signing_keys`（管理 API + 管理后台「扩展签名」页面，`DSH_EE_EXTENSION_TRUSTED_KEYS` 兼容）；官方签名标记、多环境（生产/测试）密钥标记和「供应商公钥」的审核流程仍待定。
4. 企业目录 v1 是「管理员贴入签过名的 JSON、只登记来源」（§8）。后续是否做「管理员配置 URL 自动拉取 + last-known-good 缓存」、以及目录是否由 dsh-ee 生成分发，仍待决定。
5. 作者身份怎么定义（企业账号、部门、外部供应商）？自助分发是否只对内部作者开放？
6. 回滚是否需要重新审核、是否允许回滚到任意历史版本、是否自动恢复配置？
7. 上述能力与 `plugins.md` §9 的既有设计是否一致；若调整，是否同步改那份设计稿？

## 7. 已实现的部分（#134，2026-11-13）

扩展签名的最小闭环，接口是「信任根落库」。实现在 `server/internal/extension/signature.go`（签名 / 验签 / 密钥状态）、`server/cmd/dsh-ee/cli_sign.go`（命令行）、`server/internal/api/signing_keys.go`（管理接口 + 验签合并）、`server/internal/store/signing_keys.go` 和 `server/internal/store/migrations/0032_trusted_signing_keys.sql`；使用口径见 [plugins.md](./plugins.md) §9.3。

**签名（作者侧）**

- `dsh-ee extension keygen / sign / verify`：Ed25519 detached 签名，签 `作者/名字@版本:摘要`，摘要沿用 `extension.Digest`（清单 + 接口文档的 sha256）；签名写 `signature.json`，`extension pack` 会把签名一起打进 zip。
- 私钥只在作者本地（`keygen` 生成，权限 0600），dsh-ee 不生成、不托管、不入库、不上传；服务端只保存受信任的公钥。
- 安装 / 检查计划的签名状态分 `unsigned / valid / invalid / revoked`，并带签名者的 `key_id` 和 `key_state`（active / retired / revoked），不合并成一个「可信」。

**信任根（管理侧）**

库表 `trusted_signing_keys`：`key_id`、`public_key`（PEM，PUBLIC KEY 或 CERTIFICATE，不收私钥）、`purpose`（现在只有 `extension`）、`valid_from`、`retired_at`、`revoked_at`、`created_by`、`note`、`created_at`。换下、撤销都不删行。

| 操作 | 接口 | 权限 | 审计 |
|---|---|---|---|
| 列表（含历史） | `GET /api/admin/signing-keys` | `connections.manage` 或 `settings.manage` | — |
| 新增公钥 | `POST /api/admin/signing-keys` | 同上 | `signing_key.add` |
| 换下 | `POST /api/admin/signing-keys/{key_id}/retire` | 同上 | `signing_key.retire` |
| 撤销 | `POST /api/admin/signing-keys/{key_id}/revoke` | 同上 | `signing_key.revoke` |

管理页面（`web/src/pages/admin/signing-keys.tsx`）：管理后台「能力与连接 → 扩展签名」，列表（含换下 / 撤销的历史）、粘贴 PEM 新增、换下 / 撤销（确认对话框）、只读展示 `DSH_EE_EXTENSION_TRUSTED_KEYS` 里的公钥、显示强制验签状态和「签名不是安全审计」的提示；菜单由默认菜单和迁移 `0034_admin_signing_keys_menu.sql` 补，只有 `settings.manage` 的管理员也能直接打开 `/admin/signing-keys`。i18n 中英成对。

- 验签时把库里的公钥和 `DSH_EE_EXTENSION_TRUSTED_KEYS` 合并。环境变量是只读的兼容来源；同一个 key id 以库里的状态为准——不然撤销一把同时写在 env 里的公钥不会生效。库里的改动立即生效，env 里的公钥仍然要重启才更新。
- 状态语义：
  - `active`：可以用来验新装 / 升级；
  - `retired`（换下）：不再用于新签名，历史签名仍认，安装计划里标出 `key_state: retired`；
  - `revoked`（撤销）：历史签名显示 `revoked`；**不管有没有开强制签名，新装 / 升级都拒绝**。撤销过的公钥不能重新加回来，要恢复信任请加一把新钥匙；
  - `valid_from` 在未来的：列表里是 `pending`，验签时先不用。
- 强制验签：`DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1` 时 `unsigned / invalid` 也不让装（fail closed）；这时库或 env 至少要有一把受信任公钥，一把都没有时启动只警告（可以先起服务再用管理 API 加），否则所有扩展都会被拦下。revoked 的签名在任何策略下都不让新装。
- 没有 env 也能验签：公钥只放库、不设 `DSH_EE_EXTENSION_TRUSTED_KEYS` 时，安装计划照常核对签名（有测试覆盖）。

**这次没做（仍是设计 / 待办）**

- 受信任公钥的网页管理界面已经补上（管理后台「能力与连接 → 扩展签名」，见上）；正式公钥仍由管理员手贴 PEM，没有企业 CA / KMS 接入。
- 服务扩展的清单是从对方服务实时取的，目前安装计划里显示 `unsigned`，也不会去取「随清单提供的签名」；配了强制签名时从服务地址安装会被拦下。
- DSH 插件（npm 包）签名、从 URL 拉取企业目录 / 缓存 / 离线导入（v1 只做贴入 JSON 登记，§8）、自助分发、版本回滚（§2.2–§2.4）。
- 多签（一个扩展同时用多把钥匙）、钥匙用途的审批流、官方签名的信任标记。

**边界**（同 §2.1）：签名只证明清单 / 接口文档是谁签的、没被换过，不是安全审计；撤销只影响 dsh-ee 的新装 / 升级，不会卸载已装扩展，也不改变对方系统的运行时行为。

## 8. 已实现的部分（企业目录 v1 / v1.1，2026-11-13）

企业目录 v1 是「管理员贴入一份整体签过名的 `dsh-ee-directory.json` → dsh-ee 验签 + 逐条校验 → 把条目登记到 `directory_entries`」，登记之后管理员可以对单条明确点「安装 / 升级」（`install` / `upgrade`）。**不自动安装、不自动升级、不回滚、不自动拉取目录 URL**；安装复用现有流程：扩展走安装计划 + §7 的签名策略，插件走 npm 预载 + 审核。

v1.1 在此基础上加了两件**只读**的事：

1. `GET /api/admin/directory/updates`：把**登记过的条目**和**已装的版本**对比一遍，返回 `available`（有更新）/ `current`（已是最新或企业里更新）/ `not_installed`（没装）/ `unknown`（有安装痕迹但看不出版本），条目带 `current`、`target`、`channel`、`signature`、`installed_connection`、`auto_upgrade`；**不下载、不安装、不升级**，也不改登记表。
2. format 2 的**可选**字段 `auto_upgrade`（条目级）和系统设置 `directory.autoUpgrade`（部署级，默认 `false`）：本期只登记 / 校验 / 展示，**不执行任何自动升级**；真正升级仍要管理员显式触发。

**目录格式（format 2）**

```json
{
  "format": 2,
  "directory_id": "acme-prod",
  "title": { "zh": "ACME 企业扩展目录", "en": "ACME extension directory" },
  "generated_at": "2026-11-13T08:00:00Z",
  "issuer": { "name": "ACME 平台组", "key_id": "<公钥 sha256 前 16 位十六进制>" },
  "entries": [
    {
      "kind": "extension",
      "name": "acme/keyring-itsm",
      "version": "1.2.0",
      "channel": "stable",
      "digest": "<清单（声明式连同接口文档）的 sha256 十六进制>",
      "source": { "type": "tarball", "url": "https://cdn.acme.internal/keyring-1.2.0.zip" },
      "signature": { "version": 1, "alg": "ed25519", "key_id": "…", "identity": "acme/keyring-itsm@1.2.0", "digest": "…", "signature": "…" },
      "publisher": { "username": "alice", "department": "平台组" },
      "notes": "审核过的工单扩展",
      "min_version": "0.2.0",
      "size": 1048576,
      "auto_upgrade": true
    },
    {
      "kind": "dsh-plugin",
      "name": "@acme/oa-plugin",
      "version": "2.0.0-beta.1",
      "channel": "beta",
      "digest": "sha512-<base64>",
      "source": { "type": "npm", "registry": "https://npm.acme.internal" }
    }
  ],
  "signature": { "version": 1, "alg": "ed25519", "key_id": "…", "directory_id": "acme-prod", "digest": "<canonical payload sha256>", "signature": "<base64>" }
}
```

- `kind`：`extension`（服务端扩展）或 `dsh-plugin`（npm 包）；`channel`：`stable` / `beta`。
- `digest`：扩展是 `extension.Digest` 的 sha256 十六进制；插件是 npm 的 sha512 SRI（`sha512-<base64>`）。
- `source.type`：`npm`（包名就是 `name`，可选 `registry`）或 `tarball`（固定 `https` 地址）。**不做 Git 安装**。
- `auto_upgrade`（条目内，可选，format 2 additive，默认 `false`）：声明这一条允许在部署开启 `directory.autoUpgrade` 后自动升级。本期**只登记 / 校验 / 展示**：`check` 会给一条 `entry.auto_upgrade` warning，`GET /api/admin/directory/updates` 原样返回，不会触发任何安装 / 升级。写成 `false` 与不写等价（不进 canonical payload），所以给旧目录加这个字段不影响原有目录签名。
- `signature`（条目内，可选）：扩展的签名声明，形状同扩展的 `signature.json`。目录里没有内容，所以只核对「身份/摘要和条目一致、签名者在受信任公钥里」，签名本身在安装时核对。`dsh-plugin` 的包签名还没有定义格式，只登记声明。
- 目录整体签名：Ed25519，签的是**除 `signature` 之外的 canonical JSON**（`json.Marshal` 的确定性序列化，重新缩进不影响），真正签的字节是 `dsh-ee-directory:<directory_id>:<payload sha256>`；`signature.digest` 是 payload 的 sha256。

**服务端接口**

| 操作 | 接口 | 权限 | 说明 |
|---|---|---|---|
| 校验 | `POST /api/admin/directory/check` | `connections.manage` 或 `settings.manage` | 请求体 `{"document": "<目录 JSON 文本>"}`；返回计划：`document.signature`（`unsigned / valid / invalid / revoked` + `key_state`）、逐条 `entries[]` 的字段校验与签名声明状态、`issues[]`、和已登记内容的差异 `diff`、要确认的 `sensitive[]`。**不落库** |
| 登记 | `POST /api/admin/directory/apply` | `connections.manage` 或 `settings.manage` | 同上请求体；校验通过后写 `directory_entries`。同一个 `directory_id` 覆盖登记：新增 / 更新 / 保留 / 移除。有敏感变化（移除已登记条目、同版本换内容、换来源）时返回 409，要带 `"confirm": true` 再来。**只登记，不安装**；审计 `directory.apply` |
| 列表 | `GET /api/admin/directory` | `connections.manage`、`settings.manage` 或 `plugins.manage` | 已登记的目录汇总与条目；每条带安装状态：`installed`、`installed_version`、`installed_status`（插件）、`installed_connection`（扩展）、`upgrade_available`；返回当前 tarball 白名单策略。 |
| 安装 | `POST /api/admin/directory/entries/{id}/install` | 扩展 `connections.manage`；插件 `plugins.manage` | 只对已登记条目；**管理员明确触发，不自动安装**。请求体 `{"plan_only": true}` 只出计划、不写库（扩展仍会下载并核对包，插件不启动同步），网页据此先给管理员看计划；不带 `plan_only` 才执行。扩展：按固定 tarball 地址下载、核对身份/版本/摘要（条目声明签名者时还要求包由该钥匙签名通过）后复用扩展安装计划；有敏感项要 `"confirm": true` 否则 409。插件：只接受 npm 来源的固定包名 + 固定版本，交给现有 registry 预载（`pending` 待审核，不绕过审核），返回 202 和任务 id。返回计划 / 结果；审计 `directory.install` |
| 升级 | `POST /api/admin/directory/entries/{id}/upgrade` | 同上 | 只对已装过的条目：扩展要求已装的连接存在且目录版本更新（同版本 / 降级 409），确认敏感项后走和网页升级同一条路径；插件要求企业源里已有这个包、目标版本更新，同步新版本进 `pending` 待审核。同样支持 `"plan_only": true` 先出计划。**不自动升级**；审计 `directory.upgrade` |
| 更新检查 | `GET /api/admin/directory/updates` | `connections.manage` 或 `settings.manage` | **只读**：对比已登记条目和已装版本，返回 `settings.auto_upgrade`（部署设置，本期不执行）、`summary`（各状态条数）和 `entries[]`（每条带 `entry_id / directory_id / kind / name / channel / target / current / current_source / status / signature / auto_upgrade / installed_connection / reason`）。**不下载、不安装、不升级** |

**更新检查的口径（v1.1，只读）**

「已装版本」取服务端能看到的、最接近安装结果的状态；它**不是**终端清点：

- 扩展（`kind=extension`）：扩展装成的**连接**当前生效版本里的 `extension.version`（和安装计划里的 `existing.version` 同一口径）；返回 `installed_connection`（`id / slug / title / version`）。同一种扩展有多个连接时取最早建的那个；没有连接就是 `not_installed`。
- 插件（`kind=dsh-plugin`）：企业 npm 源里这个包**已上架**的最高版本（员工能装到的版本）。员工 DSH 端真正装了哪个版本服务端看不到（只有下载记录），所以插件这一侧只是近似：企业源里没有包是 `not_installed`，有包但没有已上架版本（例如还在等审核）是 `unknown`。
- 版本比较按 semver（和 npm 源 / 安装计划同一套 `registry.CompareVersions`）：目标版本高于当前 → `available`；等于 → `current`；低于（企业里已经更新）→ 仍是 `current`，原因里写明「不需要更新 / 不能降级」。
- `signature`：条目内嵌签名声明的状态（`none` / `declared` + 按**当前**信任根重算的 `key_state`：`active / retired / revoked / unknown`），`verified` 恒为 `false`——目录里没有内容，真签名在安装时按 §7 核对。
- `auto_upgrade`：条目自己的声明；`settings.auto_upgrade` 是部署设置。本期两者都只展示，**不因此自动升级**。设置用 `PUT /api/admin/settings` 的 `directory.autoUpgrade` 修改（默认 `false`，`settings.manage`）。

- 目录签名公钥复用 §7 的信任根（`trusted_signing_keys`）：新增时 `purpose` 可以是 `directory` 或 `extension`（企业只有一套签名钥匙时不用再加一遍）；`active / retired` 可以用来登记（retired 带提示），`revoked` 拒绝。环境变量 `DSH_EE_EXTENSION_TRUSTED_KEYS` 同样作为只读兼容来源。没有配任何公钥时目录不能登记。
- tarball 来源：必须 `https`、地址里不能带用户名密码、要指向具体文件；默认只接受公网主机，拒绝 localhost、内网/链路本地/保留地址和常见内网域名（防 SSRF）；企业自己的对象存储等内网主机要显式写进 `DSH_EE_DIRECTORY_TARBALL_HOSTS`（逗号分隔，支持 `*.example.com`）白名单。登记时不下载；管理员安装扩展条目时才下载，下载前按同一策略再核对一次（默认还按 DNS 解析结果拒绝内网地址、不跟随跨主机重定向），条目可声明 `size`，超过插件 100 MiB / 扩展 8 MiB 会被拒绝。
- 上限：目录文档 2 MiB、最多 500 条；同一目录内 `kind + name + version + channel` 不能重复。
- 目录内嵌的扩展签名声明只核对「身份/摘要一致 + 签名者状态（active / retired / revoked / unknown）」；不受信任或已撤销的签名者给 warning，不阻断登记（安装时按 §7 的策略处理）。
- 安装 / 升级的边界：扩展只从条目登记的固定 tarball 地址下载（下载前按当前策略再核对地址、默认按 DNS 解析结果拒绝内网地址、不跟随跨主机重定向），内容必须与登记的版本 + 摘要一致；失败的安装都在写库前拦下，不会留下装了一半的连接。插件只接受 npm 来源，固定版本固定摘要，同步时按目录登记的 integrity 核对上游内容，对不上就失败、不入库；入库状态仍是 `pending`，要在审核页通过才对外可见，`publish`/跳过审核的入口不在这里。**不做**自动安装 / 自动升级、不做 Git 安装；页面只做「导入 / 校验 / 登记 + 看计划再安装 / 升级」，不显示私钥。
- 管理页面（`web/src/pages/admin/directory.tsx`）：管理后台「能力与连接 → 企业插件目录」，菜单由默认菜单和迁移 `0035_admin_directory_menu.sql` 补（只有 `plugins.manage` 的管理员也能直接打开 `/admin/directory`）。页面分三块：导入目录（粘贴 / 选文件 → 校验，展示目录签名、逐条 issues、差异、要确认的敏感变化 → 登记）、已登记条目列表（类型 / 名称版本 / 来源 / 发布者 / 签名声明 / 已安装 / 可升级）、安装 / 升级对话框（先调 `plan_only` 出计划给管理员看，填管理员参数 / 共享凭据 / OAuth 客户端，确认后才执行，展示结果）。私钥不进入页面也不会显示；权限按条目类型隐藏或禁用操作，失败显示服务端原因。i18n 中英成对；e2e `39-directory.mjs` 覆盖空态、非法 / 未签名目录被拦下、签名目录登记、只出计划不启动同步、确认后同步进待审核、员工无权限。
- 边界：登记目录不等于审核通过，也不等于已安装；移除条目只影响登记表，不会卸载任何东西。企业目录是新的信任根，被篡改会批量登记错来源，所以目录整体签名是硬门槛，目录不替代审核。
- `auto_upgrade` 的边界：本期只有「声明 + 只读检查」——目录条目里写 `auto_upgrade: true`、部署设置 `directory.autoUpgrade` 打开，也都**不会**自动安装 / 升级 / 改连接；`GET /api/admin/directory/updates` 的 `available` 只是提示管理员「可以升级了」。真正的升级入口属于后续版本（本期不做），现在仍走原有安装流程。
