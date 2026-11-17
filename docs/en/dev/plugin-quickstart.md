# Write a DSH plugin and publish it to the company registry

## Prerequisites

| Dependency | Version | Why it is needed |
| --- | --- | --- |
| Node | ≥ 24.9.0 | Both `dsh-primer` and `dsh-standards` require ≥24.9 in `engines`; the DSH 0.2.2 line also requires Node ≥24.9, while earlier 0.2.x lines can run on 22.19+ |
| DSH | 0.2.x line (0.2.1–0.2.2) | Run plugins and `dsh plugin add`; `dsh-primer` declares support for `>=0.2.1-0 <0.3.0`, so fill in `engines.dsh` with the range you actually verified |
| pnpm | Any recent version | Install dependencies and package |
| dsh-primer | 0.4.x | Scaffold, static checks and install smoke test (formerly `dsh-plugin-guide` 0.3.x, old CLI alias `dsh-plugin-dev`) |
| Company account | Has the "Publish capabilities" permission (`registry.publish`) | Upload plugins and request community plugins |

## Scaffold in five minutes

```bash
# 0. dsh-primer 是可安装的 DSH bundle，不是 DSH 自带命令；先装进你用的 profile
dsh plugin --profile web add dsh-primer

# 1. 生成骨架（上游模板，TypeScript）
dsh-primer new my-plugin

# 2. 本地装依赖并跑通
cd my-plugin
pnpm install
pnpm build

# 3. 自查包形状（静态检查，在本地执行）
dsh-primer check

# 4. 装进一个测试 profile 验证加载/卸载
dsh-primer verify
```

`dsh-primer` belongs to the upstream toolchain; dsh-ee only pins its version and shows usage, and does not reimplement its check logic.

## Manifest fields the company cares about most

| Field | What to write | Why the company side looks at it |
| --- | --- | --- |
| `name` | Package name under the company scope, e.g. `@company/hello` | Decides whether it can be routed to the company npm registry |
| `version` | Semantic version, one version per publish | A version number never rolls back once taken; publish conflicts reject the submission |
| `main` / `files` | The real entry point and file allowlist to publish | Avoid shipping source, tests and `.tgz` files together |
| `cordis.patch.yml` | The plugin's patch declarations for Cordis | The precheck inspects the bundle and patch structure |
| `dsh.bundle` | DSH plugin manifest (if any) | Decides whether the client recognizes this package |
| `engines.dsh` | The actually verified DSH version range | Checked against the company's DSH version line; a missing declaration gets a warning |
| `peerDependencies` | DSH-related peers | The precheck checks whether the peer range resolves |
| `license` | An SPDX identifier such as `Apache-2.0` | A missing license is a **warning** in the precheck (not blocking), but it still goes to manual review |

## What the local self-checks cover

| Command | What it checks | Where to run it |
| --- | --- | --- |
| `dsh-primer check` | Whether the package shape and entry points are complete | Author machine or author repo CI |
| `dsh-primer verify` | Smoke test of install, enable and uninstall | Author machine or author repo CI |
| `npx dsh-standards@1.6.9 check .` | Static evidence for upstream standard clauses (requires Node ≥24.9) | Author machine or author repo CI; the result is author-provided evidence |

The dsh-ee server does not execute plugin code, nor does it run these checkers; the "Share a plugin" precheck on the web only looks at the static shape of the package itself and the company admission conditions.

## Publish to the company registry

| Entry | How | Who it is for |
| --- | --- | --- |
| "Share a plugin" in the Workbench | Upload the `.tgz`, review the precheck results, then submit | First publish, when you want to see the precheck first |
| npm direct publish | `npm publish --registry <company registry>`; create the token in "My tokens" | Existing CI pipelines |
| Agent API | `POST /api/agent/packages`, called with a company token | Automated publishing |

Version numbers are taken during review and publishing; duplicate versions or conflicts are rejected. The precheck returns three conclusion fields: `can_submit` (no publish conflicts; the publish API will accept it), `will_publish` (no fail items under the open admission mode; published immediately), `requires_review` (goes to review after submission).

## FAQ

| Symptom | Common cause | What to do |
| --- | --- | --- |
| Precheck reports missing entry files | The `files` allowlist does not include build output, or `main` points to an unbuilt file | Run `pnpm build` first, then check `files` |
| Precheck has fail items | bundle / patch structure errors, suspected secrets, etc. | Fix before submitting; you can submit with fail items, but the version number is taken and review usually will not pass |
| Precheck has only warnings | Missing license, no `engines.dsh`, install scripts, etc. | Not blocking: allowlist mode goes to manual review, open mode can publish directly |
| Peer range does not resolve | Two incompatible DSH version lines are declared at the same time | Keep only one version line, or complete a range that resolves |
| Node version error on install | `engines.node` does not match the employee's Node | Fix `engines.node` according to what you actually verified (example: `^22.19.0 \|\| >=24.9.0`) |
| Cannot install inside the company | Token permissions are insufficient or npmrc is not configured for the company registry | Check the "Install capabilities" permission and `.npmrc` |

## Full text in the repository

This page is only a guide; the full text and licenses live in the repository docs (links point to the docs after merge):

- [Quickstart (docs/dev/quickstart.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/quickstart.md)
- [DSH plugin publishing (docs/dev/plugin-publish.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Errors and FAQ (docs/dev/errors-faq.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
- [Plugin author guide (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [DSH plugin development guide (docs/plugin-dev/README.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md)
- [Upstream: dsh-primer repository](https://github.com/PerryLink/dsh-primer) · [npm package page](https://www.npmjs.com/package/dsh-primer)
- [Upstream: dsh-plugin-guide (pointer page for the old 0.3.x name)](https://github.com/PerryLink/dsh-plugin-guide)
