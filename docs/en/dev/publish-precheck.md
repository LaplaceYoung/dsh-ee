# Publish entries and how to read the precheck

## Three publish entries

| Entry | Path | Good for | When the precheck runs |
| --- | --- | --- | --- |
| Web upload | "Share a plugin" in the Workbench | First publish, when you want to fix things while watching the precheck | Runs right after upload |
| npm direct publish | `npm publish --registry <company registry>` | Existing CI, publishing along a version pipeline | Runs at publish time |
| Agent API | `POST /api/agent/packages` | Automated publishing and bulk migration | Runs at publish time |

All three entries follow the same admission rules: the package first goes through a static precheck, then publish conflicts are checked, and finally the admission mode decides whether it goes live directly or enters review.

## What the precheck scans

The precheck only unpacks the package for static checks; it **does not execute plugin code**, does not write to the database, and does not reserve a version number. Results come in three levels: pass / warn / fail.

| Scan key | What it cares about | Severity |
| --- | --- | --- |
| `bundle` | `dsh.bundle` and patch files: missing, unsafe paths, files not in the package | fail for plugin packages; a warning when missing for non-plugin packages |
| `patch` | Structure of `cordis.patch.yml` (array, `insert`, `id`/`name`, duplicate ids) | structural errors are fail; name mismatches, etc. are warnings |
| `cordis` | Whether Cordis-related packages are mixed into `dependencies` | warning |
| `cordis-peer` | Whether the Cordis range in `peerDependencies` deviates from the recommended value | warning |
| `skills` | Skill manifest distributed with the package | warning (a hint that review will see it) |
| `install-scripts` | Install-time scripts (`preinstall`, etc.) | warning |
| `secrets` | Whether the package contains suspected secrets | **fail** |
| `license` | Whether an SPDX license is declared | **warning** (not blocking) |
| `engines` | Whether the plugin declares `engines.dsh` | warning |
| `dependencies` | List of dependencies outside the company scope | info |
| `size` | Package size (warning above 20 MB) | warning |

The precheck only answers "can the company safely accept this package"; it does not evaluate quality, and does not grade or rank plugins. Standards and checkers belong to the upstream toolchain, and their conclusions are author-provided evidence, not folded into the company precheck.

## How to read the results

The precheck returns three conclusion fields that match the publish API's real behavior:

| Conclusion field | Meaning |
| --- | --- |
| `can_submit` | No publish conflicts; the publish API will accept the package |
| `will_publish` | No fail items under the open admission mode; published immediately on submit |
| `requires_review` | Can be submitted, but goes into the pending-review queue |

What really decides "publish directly or enter review" is the admission mode and fail items:

| Case | Allowlist mode (default) | Open mode |
| --- | --- | --- |
| Publish conflict (package name / version taken / scope / ownership / same-name upstream package) | The publish API rejects | Same |
| Has fail items (e.g. secrets, bundle structure errors) | Can be submitted, goes to pending review; **not a rejection** | Can be submitted, goes to pending review |
| Only warnings, or all pass | Goes to pending review | No fail items; published immediately |

A version number is taken as soon as submission succeeds; rejected or unpublished versions cannot be republished, so bump the version after fixing.

## Pre-submit checklist

1. Package after `pnpm build`, and make sure the `files` allowlist includes all build output.
2. Run `dsh-primer check` and `npx dsh-standards@1.6.9 check .` locally, and keep the results in the author repo CI.
3. The package must not contain `.tgz`, source maps, real secrets in test fixtures, or absolute local paths.
4. Spell out `license`, `engines.dsh` and the DSH version line, so a mismatch does not surface only at install time.
5. If install-time scripts are needed, explain in the submission notes what the scripts do.

## FAQ

| Symptom | Common cause | What to do |
| --- | --- | --- |
| Precheck reports a publish conflict | Package name not in the company scope, version already exists, same-name upstream package | These conflicts make the publish API reject outright; rename the package or bump the version first |
| Precheck has fail items | bundle/patch structure errors, or suspected secrets detected | Fix before submitting; you can submit with fail items, but it goes to review and the version number is taken |
| Goes to pending review after submission | Allowlist mode, or fail / warning items under open mode | Fix according to the review comments; do not keep retrying the same problem with a new version number |
| Review takes a long time to conclude | Install scripts, dependencies or sensitive content need human confirmation | Add an explanation in the description, or contact an administrator |
| Wrong version after installing inside the company | Client cache, or an old version was specified | Check the version in `dsh plugin add <package>@<version>` |

## Full text in the repository

This page is only a guide; the full text on publishing, review and permissions lives in the repository docs (links point to the docs after merge):

- [DSH plugin publishing (docs/dev/plugin-publish.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/plugin-publish.md)
- [Enterprise publishing, review and permissions (docs/enterprise-publishing.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Plugin author guide (docs/plugin-authors.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-authors.md)
- [Mapping standards to admission thresholds (docs/plugin-quality.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugin-quality.md)
- [Errors and FAQ (docs/dev/errors-faq.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/errors-faq.md)
