# 版本与兼容策略（Versioning）

> 面向发布负责人、运维和接入方：dsh-ee 的版本号怎么定、发什么通道、升级/回滚的兼容窗口，以及每次发布的
> docs 版本快照怎么生成。具体的打 tag / 发 Release 操作见 [RELEASING.md](./RELEASING.md)。
>
> 以仓库里的触发条件为准：`.github/workflows/release.yml`、`Makefile`、`deploy/` 与 `server/README.md`；
> 本文只是把「版本」这件事的口径收在一页，不重复运维手册里的操作细节。

## 1. 版本号

- dsh-ee 用 **SemVer 2.0.0**：`MAJOR.MINOR.PATCH[-预发布][+构建]`。git tag 加 `v` 前缀，例如 `v0.1.0`、`v0.2.0-rc.1`。
- **tag 是版本号的唯一事实源**：仓库里没有 `VERSION` 文件，构建时由 `Makefile` 的 `VERSION` 变量经
  `-X main.version=…` 注入二进制；发布 workflow 直接把 tag 名传给 `make release VERSION=<tag>`。
- `v` 只是发布命名的前缀；比较版本时按 SemVer。同一版本号只能对应一个 commit —— 已推送的 tag **不移动、不重打**
  （`scripts/release/verify-tag.mjs` 在发布前会校验 tag 指向的 commit 与检出/主干一致）。
- 版本号只增：新版本号必须大于已有快照里的版本；`docs/versions/index.json` 的排序由生成脚本按 SemVer 维护。

语义约定（0.x 阶段也按此递增，方便以后平滑到 1.0）：

| 变更 | 版本位 | 例子 |
| --- | --- | --- |
| 破坏性变更：API 契约/配置/数据模型需要人工迁移、或明确不向后兼容 | MAJOR | `v1.0.0`；0.x 阶段用 MINOR 承载 |
| 向后兼容的新能力（新端点、新配置项、新页面） | MINOR | `v0.2.0` |
| 只修缺陷/安全修复，无新能力 | PATCH | `v0.2.1` |
| 预发布（rc/beta/alpha），接口可能再变 | 预发布后缀 | `v0.3.0-rc.1` |

## 2. 发布通道

- **stable**：`vX.Y.Z`（无预发布后缀）。给人正式使用；`stable_latest` 指向最新 stable。
- **prerelease**：`vX.Y.Z-rc.N` / `-beta.N` / `-alpha.N`。只写给试点/验证，release notes 会标明；
  `latest` 仍按 SemVer 取最高版本，`prerelease_latest` 单独指向最新预发布。
- 发布节奏不固定日历：按里程碑和需要发；发版前要求 `main` 上 CI 全绿。首个正式版本由维护者确定。

## 3. 兼容性承诺

### 3.1 数据库与升级

- 服务启动时自动执行未应用的迁移。迁移历史里有**新增表/列，也有删列、改菜单**等变更，**不保证只增不改**；
  升级前按 [operations.md](./operations.md) §6 备份数据库、`DSH_EE_DATA_DIR` 与 `.env`。
- 主密钥 `DSH_EE_SECRET_KEY` 必须保留：丢了已加密存储的上游凭据就解不开。
- 升级通常不需要手工改数据目录；多实例同时首次启动时迁移已用行锁串行化（等待/超时重试），不再要求先起一个完成迁移再扩容。

### 3.2 回滚

- 回滚 = 重新部署上一版二进制/镜像 + 保留原 `.env`；**没有自动回滚，也没有降级迁移**。
- 旧版通常忽略新版新增的表和列，但迁移不保证只增；跨版本回滚前必须用备份与当次 release notes 确认。
- 回滚后新版写入的数据可能旧版读不懂（例如统一令牌 `ee_at_` 在旧版不可用，需要改回旧令牌或前滚）。
  详细口径见 [operations.md](./operations.md) §5.4 / §5.5 与 [server/README.md](../server/README.md)。
- 因此每次发布都必须归档上一版的二进制/镜像/离线包与升级前备份；仓库不保留历史安装包。

### 3.3 HTTP / 接入契约

- 面向第三方 Harness / 自研客户端的 `/v1/*`、`/.well-known/dsh-ee`、`/mcp` 是稳定契约面，响应带
  `Dsh-EE-API-Version: 1`；客户端可在请求头带主版本。契约见 [harness-integration.md](./harness-integration.md)
  与 [l1-client.md](./l1-client.md)。
- 兼容方式优先「加字段、加端点、加可选参数」；需要破坏性变更时提升 `api_version` 主版本，而不是悄悄改语义。
- 当前 `/v1` 没有自动的弃用/Sunset 头机制，不要在 release notes 里承诺「某天自动下线」；弃用靠文档与 notes 说明。

### 3.4 令牌与扩展

- 统一令牌 `ee_at_` 按 scope 授权；旧 `ee_npm_` / `sk-ee_` 继续兼容（可配置收窄），细节与生命周期见
  [token-lifecycle.md](./token-lifecycle.md)。
- 插件/扩展清单、预检与审核的兼容边界以 [plugins.md](./plugins.md) 与 [extension-protocol.md](./extension-protocol.md)
  为准；改清单 schema 时新字段保持可选，旧版本客户端应能忽略。

## 4. 迁移 / 兼容窗口

「窗口」指旧版本、旧令牌、旧调用方式还能用多久。当前没有强制弃用日历，按下面的约定执行：

- **升级窗口**：建议在维护窗口升级（迁移期间服务不可用）；先在非生产实例用同一份数据备份验证，再升生产。
  升级前记录当前版本（`dsh-ee version`）和 `.env` 备份位置。
- **回滚窗口**：至少保留上一版部署物和升级前备份，直到新版本稳定用满一个发布周期；跨 MAJOR 不回滚二进制，
  用备份恢复到新库。
- **弃用窗口**（约定，不是自动机制）：计划移除的端点/配置/令牌形态，至少提前一个 MINOR 在 release notes 与相关
  docs 里标注，并在下一个 MAJOR 之前保留；标注时要写清替代方案。没有「声明了才允许移除」的自动化门禁，
  靠评审与 release checklist 把关。
- **安全修复**：可以只发 PATCH；若修复需要不兼容变更，按 MAJOR（或 0.x 的 MINOR）处理并在 notes 里给出迁移步骤。

## 5. docs 版本快照

站内 `/docs` 始终以 `main` 的 `docs/` 为单一事实源（构建期生成，见 [dev/README.md](./dev/README.md)）。
**版本快照**是另一件事：发布时把 tag 上「站内发布白名单」里的 Markdown 原样复制成静态目录与索引，
用于归档、离线查看和后续版本选择，不参与站内页面构建。

- 生成：`node scripts/docs/version-snapshot.mjs --tag vX.Y.Z`（`--dry-run` 只打印计划；不创建/移动 tag）。
- 校验：`node scripts/docs/version-snapshot.mjs --check`；对照本地 tag 时加 `--check-git`。
- 目录：`docs/versions/<tag>/`（`docs/` 副本 + `manifest.json` + `index.json`）与 `docs/versions/index.json`；
  索引里有 `latest`、`stable_latest`、`prerelease_latest` 和按 SemVer 排序的 `versions`。
- 内容来源是 `git show <tag>:…`，工作区未提交改动不会混进快照。快照里的相对链接必须落在发布白名单内，
  否则生成时直接失败（复制到快照目录后会断链）；跨出发布集请用仓库绝对链接。
- 发布 workflow 会为每个 tag 生成快照并作为 Release asset（`docs-versions-<tag>.tar.gz`）。
  需要仓库内长期归档时，发布后单独运行一次生成命令并提交 `docs/versions/`，再跑 `--check`；
  历史版本目录只追加、不手改。

## 6. 版本号出现在哪里

| 位置 | 形式 | 备注 |
| --- | --- | --- |
| git tag / GitHub Release | `vX.Y.Z` | 唯一事实源；发布前由 verify-tag 校验 |
| 二进制 `dsh-ee version` | `vX.Y.Z` | 由 tag 经 `-X main.version` 注入 |
| `GET /healthz` | JSON 里的 `version` | 运维探活用，见 [operations.md](./operations.md) §8 |
| `GET /.well-known/dsh-ee` | `version` 与 `api_version` | 第三方接入的版本发现，api_version 与产品版本独立 |
| docs 快照索引 | `tag` / `version` / `commit` | `docs/versions/index.json` |
| Docker 镜像 / 离线包 | `dsh-ee:vX.Y.Z` / `dsh-ee-vX.Y.Z-linux-<arch>` | 手工构建，见 [RELEASING.md](./RELEASING.md) §6 |

## 7. 相关文档

- 发布操作、checklist、回滚操作：[RELEASING.md](./RELEASING.md)
- 升级/回滚/备份的运维步骤：[operations.md](./operations.md) §5、§6
- 部署包与 `.env`：[deploy/README.md](../deploy/README.md)、[server/README.md](../server/README.md)
- 开发与 CI：[contributing.md](./contributing.md)
