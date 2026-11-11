/** DSH 的 `plugin add` 必须带 `--profile`；dsh-ee 默认接入 web profile，连接信息里可覆盖。 */
export function pluginAddCommand(name: string, profile?: string) {
  return `dsh plugin --profile ${profile || "web"} add ${name}`;
}
