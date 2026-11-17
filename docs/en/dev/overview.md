# Where to start: three development paths

## Three paths

"Development" on dsh-ee splits into three paths by deliverable: writing DSH plugins, writing dsh-ee extensions, and connecting non-DSH clients. They share the same company identity and tokens, but differ in code, checks and publishing entries.

| Path | What you deliver | Where it lands in dsh-ee | Starting tool |
| --- | --- | --- | --- |
| DSH plugin author | A DSH plugin package (tools, clients, services, commands, etc.) | Company npm registry, publish precheck, review, distribution | First `dsh plugin add dsh-primer`, then scaffold with `dsh-primer new` and self-check with `dsh-primer check` |
| dsh-ee extension author | An extension: declarative / service / local (built-in ones are implemented by dsh-ee itself) | Connections, fields & access rules, Agent skills | Scaffold with `dsh-ee extension init`, self-check with `dsh-ee extension check` |
| Non-DSH client | A client that calls models and MCP over HTTP | My tokens, model gateway, `/v1`, `/mcp` and the discovery endpoint | Create a token in "My tokens" in the Workbench first; see [Connect a third-party Harness](/docs/harness-quickstart) |

An extension comes in four forms (declarative / service / local / built-in), and the manifest can only declare the first three `kind` values; see [Write a dsh-ee extension](/docs/extension-quickstart) for details.

## First, tell plugins and extensions apart

A **DSH plugin** is installed in the DSH client and built and verified by its author on their own machine; dsh-ee handles company-side admission, hosting and distribution.

A **dsh-ee extension** is the integration unit that connects to the dsh-ee server. It is managed by dsh-ee's own CLI, and administrators configure it in "Connections" and "Fields & access rules".

| Aspect | DSH plugin | dsh-ee extension |
| --- | --- | --- |
| Where it runs | The machine running the DSH client | Requests from inside the dsh-ee process (declarative), the company's own service (service), employee machines (local) |
| Manifest file | `package.json` and `cordis.patch.yml` | `dsh-ee.extension.json` |
| Publishing entry | The company npm registry, or "Share a plugin" in the Workbench | Upload a zip for declarative; administrators fill in the service address for service extensions; deliver an npm package plus manifest for local |
| Who can install | Employees with the "Install capabilities" permission | Configured by administrators in connections and rules |

## Suggested reading order

1. Publishing a plugin for the first time: [Write a DSH plugin and publish it to the company registry](/docs/plugin-quickstart)
2. Writing an extension for the first time: [Write a dsh-ee extension: manifest, checks and calls](/docs/extension-quickstart)
3. Connecting a third-party Harness or your own client: [Connect a third-party Harness or your own client](/docs/harness-quickstart)
4. Confirming permission boundaries: [Permissions and access rules: a quick reference](/docs/access-rules)
5. Wanting to know what you will be asked before submitting: [Publish entries and how to read the precheck](/docs/publish-precheck)
6. Working out where standards and checks meet: [Standards and checks: ENTRY / FLOW / BOOST](/docs/quality-standards)

## What else to prepare

| Item | Notes |
| --- | --- |
| Account and permissions | Publishing plugins needs the "Publish capabilities" permission (`registry.publish`); extension development materials need "Develop extensions" (`extensions.develop`) |
| Company registry address | Get the npm registry address, token configuration and `setup.sh` usage from the "Setup guide" |
| Local toolchain | Node ≥24.9 (the `engines` requirement of `dsh-primer` and `dsh-standards`) and pnpm; Go 1.27+ only for service extensions |
| Network | On an intranet deployment, GitHub and the public npm may be unreachable; confirm dependencies can be fetched first, and use "upstream preload" when necessary |

## Site guides and repository docs

The site pages only give dsh-ee-side summaries and entry points and do not copy upstream text; repository docs may be quoted or adapted under their respective licenses (CC BY 4.0 requires keeping TASL and a Change Notice; Apache-2.0 requires keeping LICENSE/NOTICE). For the text, licenses and upstream versions, the repository docs and upstream repositories are authoritative (links point to the docs after merge):

| Content | Where |
| --- | --- |
| Development docs index (quickstart / extensions / Harness / publishing / standards / errors & FAQ) | [docs/dev/README.md (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/README.md) |
| DSH plugin development guide (the local reference layer for the community guides) | [docs/plugin-dev/README.md (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/README.md) |
| Standards and checker author reference (ENTRY / FLOW / BOOST, DS-C, checker) | [docs/plugin-dev/standards/README.md (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) |
| Extension protocol v1 overview | [docs/extension-protocol/README.md (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md) |
| Enterprise publishing, review and permissions | [docs/enterprise-publishing.md (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md) |

Upstream tools are pinned: `dsh-primer` ([npm package page](https://www.npmjs.com/package/dsh-primer)) and `dsh-standards@1.6.9` ([npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)); dsh-ee only pins versions and shows usage, it does not reimplement upstream check logic.
