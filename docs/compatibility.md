# DSH compatibility matrix

The real-DSH scenarios under `e2e/dsh/` run against an installed DSH binary (`DSH_BIN`, default `.tools/dsh-rc5/node_modules/.bin/dsh`). Every `result.json` now carries `dshVersion`, and the run writes `work/dsh-version.txt`, so a matrix entry can be traced to the exact host version instead of a hand-written claim.

## Run one version

```sh
# build dsh-ee and prepare a TiDB first; see e2e/README.md
export DSH_BIN=/path/to/dsh
export CHROME=/path/to/chrome
export DSH_EE_TEST_DSN='root:@tcp(127.0.0.1:4001)/'
node e2e/dsh/scenarios/run.mjs
jq '{dshVersion, status}' .tools/dsh-scenarios/<run>/a1/result.json
```

Repeat with `DSH_BIN` pointing at another version and a fresh `SCENARIO_WORK`.

## Matrix

Fill a row only after the scenario actually ran; keep the date and the failing assertion/step in the last column. `n/a` is for scenarios that do not exist on that DSH version yet.

| DSH version | Date | Scenario | Result | Notes / first failing step |
|---|---|---|---|---|
| 0.2.1-rc.5 | 2026-11-11 | `run.mjs` (A1/A2/A3) | 待跑 | 本机 CLI 版本，尚未在本次会话跑完整场景 |
| 0.2.2-rc.1 | | `run.mjs` | 待跑 | |
| 0.2.2 | | `run.mjs` | 待跑 | 维护者口径的 v0.1 基准版本 |

## Why this exists

`docs/roadmap.md` stage A5 asks for a compatibility matrix based on DSH 0.2.2; before this change the harness spawned DSH without recording which version it was, so two runs could not be compared and a failure could not be attributed. The version is now captured before any server/scenario starts; `unknown` means `DSH_BIN --version` failed and the run should not be used as matrix evidence.

The harness only reads the version; it does not install DSH or run anything from the package under test.
