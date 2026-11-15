# Standards and checks: ENTRY / FLOW / BOOST

## What each of the three layers answers

"Standards" come from upstream dsh-plugin-guide / dsh-primer and dsh-standards. dsh-ee **does not run checkers built into the server**; repository docs may quote or adapt upstream text under their respective licenses — CC BY 4.0 requires keeping TASL, a Change Notice and a same-view disclaimer; Apache-2.0 requires keeping LICENSE/NOTICE. The site guides only write summaries and links, while the adapted text and per-document attribution live in the repository at [`docs/plugin-dev/standards/` (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md) and [`docs/dev/standards.md` (available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md).

| Layer | Question it answers | Who uses it | Where it sits in dsh-ee |
| --- | --- | --- | --- |
| ENTRY | Whether a plugin's minimal shape holds (entry, manifest, license) | Authors, author repo CI | Aligned with the package-shape check in the publish precheck |
| FLOW | Whether the plugin's flow holds up in real use (install, enable, uninstall, failure recovery) | Authors, testers | The example guides explain how to reproduce it |
| BOOST | Whether experience and composability are in place (commands, rendering, skills, multiple surfaces) | Authors, reviewers | Examples are grouped by the surfaces involved |

## How ENTRY / FLOW / BOOST are used on the company side

| Use | Concrete practice | What not to do |
| --- | --- | --- |
| Author-provided evidence | Run the upstream checker (`dsh-standards@1.6.9`, requires Node ≥24.9) in the author repo CI, and pin the command and version | Do not treat upstream check results as the company's admission conclusion |
| Company admission | After upload, read dsh-ee's own precheck details (pass / warn / fail and publish conflicts) | Do not use upstream checks in place of the precheck |
| Review reference | During review, ask questions against the examples and standard clauses | Do not grade or rank plugins or examples on the page |
| Learning path | Read the code from beginner to advanced following the examples | Do not treat examples as official recommendations |

## What dsh-ee does not do

- It does not run third-party checkers on the server, and does not provide a sandbox where checks can run.
- It does not turn upstream check results into quality levels, scores or rankings.
- It does not present adapted standards text as upstream original text: quoting or adapting must keep the source, the license (CC BY 4.0 / Apache-2.0) and a Change Notice, and adapted documents carry per-document attribution.
- It does not promise "officially recommended": examples only show which kind of capability they demonstrate.

## How authors leave evidence

1. Run the pinned `dsh-standards@1.6.9` (Node ≥24.9) locally, and put the commands into the author repo CI.
2. Save the JSON or HTML report as a build artifact, and record the generation time and host version.
3. Reference the report location in the release notes instead of writing only a conclusion such as "checked".
4. Rerun after an upstream standards upgrade; mark the old report with its version and do not mix them.

```bash
# 固定版本，避免 npx 同名包冲突
npx dsh-standards@1.6.9 check .
npx dsh-standards@1.6.9 check . --json > report.json
```

How to read the checker's exit codes, report fields and "not-run / NA / static does not mean pass" is documented in the author reference in the repository (see the links below).

## Relationship to the example pages

The example pages show "one complete runnable minimal corridor": each example states what you can learn, the prerequisites, the pinned version and the install command, and the detail page gives the source and license. Examples do not come with check conclusions, and the page does not rank examples by check results.

## Full text in the repository

This page is only a guide; the standards text, clause index and checker usage live in the repository docs and upstream repositories (links point to the docs after merge):

- [Standards and checks (docs/dev/standards.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/standards.md)
- [Standards and checker author reference (docs/plugin-dev/standards/README.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/README.md)
- [Checker commands and report fields (docs/plugin-dev/standards/checker-cli.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/checker-cli.md)
- [Evidence boundaries and common misreadings (docs/plugin-dev/standards/evidence-and-misreadings.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-dev/standards/evidence-and-misreadings.md)
- [Mapping standards to admission thresholds (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Upstream: dsh-standards repository](https://github.com/PerryLink/dsh-standards) · [npm v1.6.9](https://www.npmjs.com/package/dsh-standards/v/1.6.9)
- [Upstream: dsh-primer (DSH plugin development guide and toolchain)](https://github.com/PerryLink/dsh-primer)
