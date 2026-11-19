package api

import (
	"net/http"
	"net/url"
	"strings"
)

// 员工电脑的一键接入脚本：让 DSH 从 dsh-ee 安装插件，并通过设备码或手动令牌登录。
// npm 源配置只写进 DSH profile 目录的 .npmrc（DSH 跑 pnpm 时的工作目录），不动 ~/.npmrc，
// 员工自己的 npm / pnpm 不受影响；DSH_EE_GLOBAL_NPMRC=1 时才额外写全局配置。
// 设备登录只写 npm 配置与 DSH 待导入文件；手动令牌继续写 .env 以兼容 CI 和旧流程。
//
//	curl -fsSL <dsh-ee>/setup.sh | sh
//	curl -fsSL <dsh-ee>/setup.sh | sh -s -- --token ee_npm_…
//	irm <dsh-ee>/setup.ps1 | iex

// connectSetup 是脚本里安装接入插件要用的信息；插件还没发布或被关掉时 pkg 为空。
func (s *Server) connectSetup(r *http.Request) (pkg string, market bool) {
	set, err := s.appSettings(r.Context())
	if err != nil || !set.Connect.Enabled {
		return "", false
	}
	var st connectState
	if _, err := s.store.GetSettingJSON(r.Context(), connectStateKey, &st); err != nil || st.Package != connectPackage(set) || st.Version == "" {
		return "", set.Connect.Market
	}
	return st.Package, set.Connect.Market
}

func (s *Server) setupVars(r *http.Request) (base, registry, authKey string, scopes []string, err error) {
	base = s.publicBase(r)
	registry = base + "/npm/"
	u, err := url.Parse(registry)
	if err != nil {
		return
	}
	authKey = "//" + u.Host + u.Path + ":_authToken"
	set, err := s.appSettings(r.Context())
	if err != nil {
		return
	}
	scopes = set.CompanyScopes()
	return
}

// ps1Lang 是 setup.ps1 的文案语言。默认中文：现有 Windows 用户看到的是中文，
// e2e/29-release-smoke.mjs 也断言默认脚本里有「dsh-ee 接入脚本」。
// 只有显式 ?lang=en 才输出英文；和 setup.sh 的取舍一致，不看 Accept-Language，
// 避免浏览器默认语言改变默认输出。
type ps1Lang string

const (
	ps1LangZh ps1Lang = "zh"
	ps1LangEn ps1Lang = "en"
)

// ps1LangFromRequest 只认显式 lang=en（大小写不敏感）；缺省或非法值一律回退中文。
func ps1LangFromRequest(r *http.Request) ps1Lang {
	if strings.EqualFold(r.URL.Query().Get("lang"), "en") {
		return ps1LangEn
	}
	return ps1LangZh
}

// profileNpmrcShell 定义 write_profile_npmrc <profile 目录>：替换 dsh-ee 管的几行（registry、作用域、令牌），
// 其它行原样保留；以及 check_token：用 /api/dsh/whoami 校验令牌，令牌经 stdin 传给 curl，不出现在进程参数里。
const profileNpmrcShell = `
write_profile_npmrc() {
  dir="$1"
  file="$dir/.npmrc"
  mkdir -p "$dir"
  touch "$file"
  chmod 600 "$file"
  keys="registry $AUTH_KEY"
  for scope in $SCOPES; do keys="$keys $scope:registry"; done
  awk -v keys="$keys" 'BEGIN { n = split(keys, k, " "); for (i = 1; i <= n; i++) drop[k[i]] = 1 } { key = $0; sub(/=.*/, "", key) } !(key in drop)' "$file" > "$file.tmp"
  {
    printf '%s=%s\n' 'registry' "$REGISTRY"
    for scope in $SCOPES; do printf '%s:registry=%s\n' "$scope" "$REGISTRY"; done
    printf '%s=%s\n' "$AUTH_KEY" "$TOKEN"
  } >> "$file.tmp"
  mv "$file.tmp" "$file"
  chmod 600 "$file"
  msg_wrote_npmrc "$file"
}
check_token() {
  printf 'header = "Authorization: Bearer %s"\n' "$TOKEN" | curl -fsS -K - "$BASE/api/dsh/whoami" >/dev/null
}
hint_global_npmrc() {
  if [ "${DSH_EE_GLOBAL_NPMRC:-}" != "1" ] && [ -f "$HOME/.npmrc" ] && grep -qF "$REGISTRY" "$HOME/.npmrc"; then
    msg_old_global_npmrc "$REGISTRY"
  fi
}
`

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

func psQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

// ps1Text 是 setup.ps1 里所有面向用户的文案：提示、注释、错误信息。
// 脚本的命令、变量、逻辑与退出码中英共用，切语言只替换这里的文案；
// 默认中文必须与加 lang 参数之前逐字一致（e2e 与 Windows 用户现状都依赖它）。
type ps1Text struct {
	header          string
	noDeviceLogin   string
	noNpm           string
	deviceStartFail string
	browserHint     string
	deviceCode      string
	waitConfirm     string
	devicePollFail  string
	deviceTimeout   string
	loginOK         string
	tokenFormat     string
	tokenInvalid    string
	tokenValid      string
	profilesEmpty   string
	npmrcComment    string
	bomComment      string
	npmrcWritten    string
	globalNpmrcDone string
	envBomComment1  string
	envBomComment2  string
	envAclComment   string
	envTokenWritten string
	envPendingSaved string
	envOldTokenHint string
	noConnectPkg    string
	indexHint       string
	pnpmMissing     string
	pluginInstalled string
	done            string
	autoSync        string

	// 插件装失败（多半是 profile 里以前从公共 npm 装的插件在 dsh-ee 上没审核过，#239）
	pluginAddFailed   string
	nativeExitComment string
}

func ps1TextFor(lang ps1Lang) ps1Text {
	if lang == ps1LangEn {
		return ps1TextEn
	}
	return ps1TextZh
}

var ps1TextZh = ps1Text{
	header:          `# dsh-ee 接入脚本：默认通过浏览器设备登录；-Token 保留 CI 与旧流程。`,
	noDeviceLogin:   `当前环境不能进行浏览器设备登录，请使用 -Token <令牌> 或设置 DSH_EE_TOKEN。`,
	noNpm:           `没有找到 npm：请先安装 Node.js。`,
	deviceStartFail: `设备登录发起失败：`,
	browserHint:     `请在浏览器中确认登录（已尝试自动打开）：`,
	deviceCode:      `确认码：$($flow.user_code)`,
	waitConfirm:     `等待确认…（Ctrl-C 取消）`,
	devicePollFail:  `设备登录失败：$($pollError.error)`,
	deviceTimeout:   `设备登录超时，请重新运行 setup`,
	loginOK:         `✔ 已登录 $($login.user.username)（$($login.user.department)）`,
	tokenFormat:     `令牌格式不对`,
	tokenInvalid:    `令牌无效或连不上 dsh-ee`,
	tokenValid:      `令牌有效，当前账号：$($me.username)`,
	profilesEmpty:   `DSH_PROFILES 不能为空`,
	npmrcComment:    `只写 DSH profile 目录的 .npmrc（DSH 跑 pnpm 时的工作目录），不动 ~/.npmrc`,
	bomComment:      `.NET 默认写无 BOM 的 UTF-8；Set-Content -Encoding utf8 在 PowerShell 5 会加 BOM`,
	npmrcWritten:    `已写入 $NpmrcFile（只对 DSH 生效，不改 ~/.npmrc）`,
	globalNpmrcDone: `已按 DSH_EE_GLOBAL_NPMRC=1 同时写入 ~/.npmrc`,
	envBomComment1:  `.NET 默认写无 BOM 的 UTF-8；Set-Content -Encoding utf8 在 PowerShell 5 会加 BOM，`,
	envBomComment2:  `Node 的 process.loadEnvFile 会把 BOM 当成变量名的一部分，DSHEE_TOKEN 就读不到。`,
	envAclComment:   `.env 里也是接入令牌，权限和 .npmrc / token.pending 保持一致`,
	envTokenWritten: "已把接入令牌写进 $EnvFile（" + connectTokenEnv + "）",
	envPendingSaved: `令牌已保存到 $Pending，DSH 启动后会导入凭据库并删除此文件`,
	envOldTokenHint: `提示：已保留 .env 中的旧令牌；凭据库优先，如不再使用请手动删除该行。`,
	noConnectPkg:    `管理员还没有发布 DSH 接入插件；发布后重新运行本脚本即可接入企业技能与系统。`,
	indexHint:       `现在可以先在 DSH 插件页 → 发现 → 添加索引源，填入`,
	pnpmMissing:     `安装 DSH 插件需要 pnpm：先运行 npm install -g pnpm，再重新运行本脚本`,
	pluginInstalled: `已在 DSH 的 $ProfileName profile 里装好 $ConnectPkg`,
	done:            `完成。重启 DSH（dsh web）后，企业技能、提示词、微应用与企业系统就会出现。`,
	autoSync:        `以后在 $Base/hub 启用新的能力，DSH 会自动同步。`,

	pluginAddFailed:   `在 DSH 的 $ProfileName profile 里装 $ConnectPkg 失败，上面是 DSH 的原始输出。如果里面有 ERR_PNPM_FETCH_404 或 404 Not Found：这个 profile 里以前从公共 npm 装过的某个插件，在 dsh-ee 上还没有审核通过。请管理员在 dsh-ee 的「待审核」里放行它；或者先运行 dsh plugin --profile $ProfileName remove <404 那一行里的包名，%2F 换成 />，再重新运行本脚本`,
	nativeExitComment: `dsh / npx 是外部命令，失败了 PowerShell 也不会停下来：要自己看退出码，不然会照样打出「完成」`,
}

var ps1TextEn = ps1Text{
	header:          `# dsh-ee setup script: browser device login by default; -Token for CI and legacy flows.`,
	noDeviceLogin:   `This environment cannot do browser device login. Use -Token <token> or set DSH_EE_TOKEN.`,
	noNpm:           `npm not found: install Node.js first.`,
	deviceStartFail: `Failed to start device login: `,
	browserHint:     `Confirm the login in the browser (we tried to open it automatically):`,
	deviceCode:      `Verification code: $($flow.user_code)`,
	waitConfirm:     `Waiting for confirmation... (Ctrl-C to cancel)`,
	devicePollFail:  `Device login failed: $($pollError.error)`,
	deviceTimeout:   `Device login timed out; run setup again`,
	loginOK:         `✔ Signed in as $($login.user.username) ($($login.user.department))`,
	tokenFormat:     `Invalid token format`,
	tokenInvalid:    `Invalid token, or dsh-ee is unreachable`,
	tokenValid:      `Token is valid. Current account: $($me.username)`,
	profilesEmpty:   `DSH_PROFILES must not be empty`,
	npmrcComment:    `Only write the .npmrc in the DSH profile directory (the working dir where DSH runs pnpm); leave ~/.npmrc alone`,
	bomComment:      `.NET writes UTF-8 without BOM by default; Set-Content -Encoding utf8 adds a BOM on PowerShell 5`,
	npmrcWritten:    `Wrote $NpmrcFile (takes effect only for DSH; ~/.npmrc untouched)`,
	globalNpmrcDone: `Also wrote ~/.npmrc because DSH_EE_GLOBAL_NPMRC=1`,
	envBomComment1:  `.NET writes UTF-8 without BOM by default; Set-Content -Encoding utf8 adds a BOM on PowerShell 5,`,
	envBomComment2:  `Node process.loadEnvFile treats the BOM as part of the variable name, so DSHEE_TOKEN would not load.`,
	envAclComment:   `.env also holds the access token; keep the same ACL as .npmrc / token.pending`,
	envTokenWritten: "Wrote the access token to $EnvFile (" + connectTokenEnv + ")",
	envPendingSaved: `Token saved to $Pending; DSH will import it into the credential store on startup and delete this file`,
	envOldTokenHint: `Note: kept the old token in .env; the credential store takes precedence, so delete that line manually if it is no longer needed.`,
	noConnectPkg:    `The admin has not published the DSH connect plugin yet; rerun this script after it is published to connect company skills and systems.`,
	indexHint:       `For now, open DSH Plugins -> Discover -> Add index source and fill in`,
	pnpmMissing:     `Installing the DSH plugin requires pnpm: run npm install -g pnpm first, then rerun this script`,
	pluginInstalled: `Installed $ConnectPkg in the DSH $ProfileName profile`,
	done:            `Done. After restarting DSH (dsh web), company skills, prompts, micro apps and enterprise systems will show up.`,
	autoSync:        `New capabilities enabled at $Base/hub will sync to DSH automatically.`,

	pluginAddFailed:   `Installing $ConnectPkg in the DSH $ProfileName profile failed; DSH output is above. If it shows ERR_PNPM_FETCH_404 or 404 Not Found, a plugin installed earlier in this profile from the public npm registry has not been approved on this dsh-ee. Ask an admin to approve it in the dsh-ee review queue, or run dsh plugin --profile $ProfileName remove <package name from the 404 line, with %2F replaced by /> and rerun this script`,
	nativeExitComment: `dsh / npx are native commands; PowerShell does not stop when they fail, so check the exit code or it would still print Done`,
}

func (s *Server) handleSetupSh(w http.ResponseWriter, r *http.Request) {
	base, registry, authKey, scopes, err := s.setupVars(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	lang := setupLang(r)
	script := setupShellHeader(lang) + `set -eu
umask 077
` + setupShellPrelude(lang) + `BASE=` + shellQuote(base) + `
REGISTRY=` + shellQuote(registry) + `
AUTH_KEY=` + shellQuote(authKey) + `
INDEX=` + shellQuote(base+"/index/dsh-index.json") + `
SCOPES=` + shellQuote(strings.Join(scopes, " ")) + `
TOKEN=""
if [ "${1:-}" = "--token" ]; then
  if [ "$#" -lt 2 ] || [ -z "$2" ]; then
    msg_usage
    exit 1
  fi
  TOKEN="$2"
elif [ "$#" -gt 0 ]; then
  case "$1" in
    ee_*|sk-ee-*) TOKEN="$1" ;;
    *) msg_unknown_arg "$1"; exit 1 ;;
  esac
fi
if [ -z "$TOKEN" ] && [ -n "${DSH_EE_TOKEN:-}" ]; then TOKEN="$DSH_EE_TOKEN"; fi
DEVICE_LOGIN=0
if [ -z "$TOKEN" ]; then
  if [ "${CI:-}" = "1" ] || { [ "${DSH_EE_SETUP_INTERACTIVE:-}" != "1" ] && [ ! -t 0 ] && [ ! -r /dev/tty ]; }; then
    msg_no_device_login
    exit 1
  fi
  DEVICE_LOGIN=1
fi
` + profileNpmrcShell + s.desktopShell(r, lang) + `
if ! command -v npm >/dev/null 2>&1 || ! command -v node >/dev/null 2>&1; then
  msg_no_node
  exit 1
fi
DSH_DIR="${DSH_HOME:-$HOME/.dsh}"
PENDING_DIR="$DSH_DIR/dsh-ee"
PENDING_FILE="$PENDING_DIR/token.pending"
if [ "$DEVICE_LOGIN" -eq 1 ]; then
  DSH_EE_SETUP_BASE="$BASE"
  DSH_EE_SETUP_LANG="$SETUP_LANG"
  export DSH_EE_SETUP_BASE DSH_EE_SETUP_LANG
  node -e ` + shellQuote(setupDeviceJS(lang)) + `
  TOKEN=$(cat "$PENDING_FILE")
fi
case "$TOKEN" in
  *[!A-Za-z0-9._~-]*) msg_bad_token_format; exit 1 ;;
esac
if ! check_token; then
  msg_bad_token
  exit 1
fi
msg_token_ok
PROFILES="${DSH_PROFILES:-` + connectProfile + `}"
if [ -z "$(printf '%s' "$PROFILES" | tr -d '[:space:]')" ]; then
  msg_profiles_empty
  exit 1
fi
for profile in $PROFILES; do
  write_profile_npmrc "$DSH_DIR/profiles/$profile"
done
if [ "${DSH_EE_GLOBAL_NPMRC:-}" = "1" ]; then
  npm config set registry "$REGISTRY"
  npm config set "$AUTH_KEY" "$TOKEN"
  for scope in $SCOPES; do
    npm config set "$scope:registry" "$REGISTRY"
  done
  msg_global_npmrc_written
else
  hint_global_npmrc
fi
` + s.connectShell(r)

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	// 输出随 Accept-Language 变化，避免中间缓存把一种语言的脚本发给另一种语言的员工。
	w.Header().Add("Vary", "Accept-Language")
	_, _ = w.Write([]byte(script))
}

// handleSetupPs1 生成 Windows 一键接入脚本。默认中文；显式 ?lang=en 生成全英文脚本。
// 不用 Accept-Language：默认输出不能被浏览器语言改变（e2e 断言中文 marker）。
func (s *Server) handleSetupPs1(w http.ResponseWriter, r *http.Request) {
	base, registry, authKey, scopes, err := s.setupVars(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	txt := ps1TextFor(ps1LangFromRequest(r))
	var sc []string
	for _, x := range scopes {
		sc = append(sc, psQuote(x))
	}
	script := txt.header + `
function Setup-DshEE {
  param([string]$Token)
  $ErrorActionPreference = 'Stop'
  $Base = ` + psQuote(base) + `
  $Registry = ` + psQuote(registry) + `
  $AuthKey = ` + psQuote(authKey) + `
  $Index = ` + psQuote(base+"/index/dsh-index.json") + `
  if (-not $Token -and $env:DSH_EE_TOKEN) { $Token = $env:DSH_EE_TOKEN }
  $DeviceLogin = -not $Token
  if ($DeviceLogin -and ($env:CI -eq '1' -or ($env:DSH_EE_SETUP_INTERACTIVE -ne '1' -and -not [Environment]::UserInteractive))) {
    Write-Error '` + txt.noDeviceLogin + `'
    return
  }
  if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error '` + txt.noNpm + `'
    return
  }
  $DshDir = if ($env:DSH_HOME) { $env:DSH_HOME } else { Join-Path $HOME '.dsh' }
  $PendingDir = Join-Path $DshDir 'dsh-ee'
  $Pending = Join-Path $PendingDir 'token.pending'
  if ($DeviceLogin) {
    $dshVersion = ''
    if (Get-Command dsh -ErrorAction SilentlyContinue) { $dshVersion = (dsh --version | Select-Object -First 1) }
    $body = @{ client_name = $env:COMPUTERNAME; client_os = [System.Environment]::OSVersion.VersionString; client_version = $dshVersion } | ConvertTo-Json -Compress
    try { $flow = Invoke-RestMethod -Method Post -Uri "$Base/api/device/code" -ContentType 'application/json' -Body $body }
    catch { Write-Error "` + txt.deviceStartFail + `$_"; return }
    Write-Host '` + txt.browserHint + `'
    Write-Host "  $($flow.verification_uri_complete)"
    Write-Host "` + txt.deviceCode + `"
    Write-Host '` + txt.waitConfirm + `'
    try { Start-Process $flow.verification_uri_complete } catch {}
    $interval = [Math]::Max(1, [int]$flow.interval)
    $deadline = [DateTime]::UtcNow.AddSeconds([Math]::Max(1, [int]$flow.expires_in))
    $login = $null
    while ([DateTime]::UtcNow -lt $deadline) {
      Start-Sleep -Seconds $interval
      try {
        $login = Invoke-RestMethod -Method Post -Uri "$Base/api/device/token" -ContentType 'application/json' -Body (@{ device_code = $flow.device_code } | ConvertTo-Json -Compress)
      } catch {
        $pollError = $null
        if ($_.ErrorDetails.Message) { try { $pollError = $_.ErrorDetails.Message | ConvertFrom-Json } catch {} }
        if ($pollError.error -eq 'slow_down' -or $pollError.error -eq 'rate_limited') { $interval += 5; continue }
        if ($pollError.error -eq 'authorization_pending') { continue }
        Write-Error "` + txt.devicePollFail + `"
        return
      }
      if ($login.access_token) { break }
    }
    if (-not $login.access_token) { Write-Error '` + txt.deviceTimeout + `'; return }
    New-Item -ItemType Directory -Force -Path $PendingDir | Out-Null
    [System.IO.File]::WriteAllText($Pending, $login.access_token + [Environment]::NewLine)
    icacls $PendingDir /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)(F)" | Out-Null
    icacls $Pending /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    $Token = $login.access_token
    Write-Host "` + txt.loginOK + `"
  }
  if ($Token -notmatch '^[A-Za-z0-9._~-]+$') { Write-Error '` + txt.tokenFormat + `'; return }
  try { $me = Invoke-RestMethod -Uri "$Base/api/dsh/whoami" -Headers @{ Authorization = "Bearer $Token" } }
  catch { Write-Error '` + txt.tokenInvalid + `'; return }
  Write-Host "` + txt.tokenValid + `"
  $Scopes = @(` + strings.Join(sc, ", ") + `)
  $rawProfiles = if ($env:DSH_PROFILES) { $env:DSH_PROFILES } else { '` + connectProfile + `' }
  $Profiles = @($rawProfiles -split '\s+' | Where-Object { $_ })
  if ($Profiles.Count -eq 0) { Write-Error '` + txt.profilesEmpty + `'; return }
  foreach ($ProfileName in $Profiles) {
    # ` + txt.npmrcComment + `
    $ProfileDir = Join-Path (Join-Path $DshDir 'profiles') $ProfileName
    $NpmrcFile = Join-Path $ProfileDir '.npmrc'
    New-Item -ItemType Directory -Force -Path $ProfileDir | Out-Null
    $drop = @('registry', $AuthKey) + ($Scopes | ForEach-Object { "${_}:registry" })
    $lines = @()
    if (Test-Path $NpmrcFile) { $lines = @(Get-Content $NpmrcFile -Encoding UTF8 | Where-Object { $drop -notcontains ($_ -split '=', 2)[0] }) }
    $lines += "registry=$Registry"
    foreach ($scope in $Scopes) { $lines += "${scope}:registry=$Registry" }
    $lines += "$AuthKey=$Token"
    # ` + txt.bomComment + `
    [System.IO.File]::WriteAllLines($NpmrcFile, [string[]]$lines)
    icacls $NpmrcFile /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    Write-Host "` + txt.npmrcWritten + `"
  }
  if ($env:DSH_EE_GLOBAL_NPMRC -eq '1') {
    npm config set registry $Registry
    npm config set $AuthKey $Token
    foreach ($scope in $Scopes) { npm config set "${scope}:registry" $Registry }
    Write-Host '` + txt.globalNpmrcDone + `'
  }
` + s.connectPowerShell(r, txt) + `}
` + setupTokenArgPowerShell() + `Setup-DshEE -Token $tokenArg
`
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	// 不加 BOM：iwr | iex 会把 BOM 当成脚本的第一个字符；iwr 按 Content-Type 的 charset 解码
	_, _ = w.Write([]byte(script))
}

// setupTokenArgPowerShell 把命令行上的 -Token 传进 Setup-DshEE。
// irm | iex 不能传参；但 CI / 旧流程会用 `powershell -File setup.ps1 -Token x`，
// 这种写法参数只在脚本级 $args 里，不会自动进函数参数。
func setupTokenArgPowerShell() string {
	return `$tokenArg = ''
for ($i = 0; $i -lt $args.Count; $i++) {
  $arg = [string]$args[$i]
  if ($arg -eq '-Token' -and ($i + 1) -lt $args.Count) { $tokenArg = [string]$args[$i + 1]; break }
  if ($arg -like '-Token=*') { $tokenArg = $arg.Substring(7); break }
}
`
}

// connectShell 是 setup.sh 的后半段：保存手动令牌或提示待导入文件，再装接入插件。
// 装插件失败时（#239：profile 里以前从公共 npm 装过、在 dsh-ee 上没审核过的插件让 pnpm 404），set -e 会直接退出、
// 只留下 pnpm 的报错；所以先接住失败，说清楚最常见的原因和怎么办，再以失败退出。
func (s *Server) connectShell(r *http.Request) string {
	pkg, market := s.connectSetup(r)
	tail := `
# ---- DSH ----
ENV_FILE="$DSH_DIR/.env"
if [ "$DEVICE_LOGIN" -eq 0 ]; then
  mkdir -p "$DSH_DIR"
  touch "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  grep -v '^` + connectTokenEnv + `=' "$ENV_FILE" > "$ENV_FILE.tmp" || true
  echo "` + connectTokenEnv + `=$TOKEN" >> "$ENV_FILE.tmp"
  mv "$ENV_FILE.tmp" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  msg_saved_env "$ENV_FILE" '` + connectTokenEnv + `'
else
  chmod 700 "$PENDING_DIR"
  chmod 600 "$PENDING_FILE"
  msg_saved_pending "$PENDING_FILE"
  if [ -f "$ENV_FILE" ] && grep -q '^` + connectTokenEnv + `=' "$ENV_FILE"; then
    REMOVE_OLD=0
    if [ "${DSH_EE_SETUP_INTERACTIVE:-}" != "1" ] && [ -r /dev/tty ] && [ -w /dev/tty ]; then
      msg_ask_remove_old "$ENV_FILE"
      read -r answer < /dev/tty || answer=""
      case "$answer" in y|Y|yes|YES) REMOVE_OLD=1 ;; esac
    fi
    if [ "$REMOVE_OLD" -eq 1 ]; then
      grep -v '^` + connectTokenEnv + `=' "$ENV_FILE" > "$ENV_FILE.tmp" || true
      mv "$ENV_FILE.tmp" "$ENV_FILE"
      chmod 600 "$ENV_FILE"
      msg_old_token_removed
    else
      msg_old_token_kept
    fi
  fi
fi
`
	if pkg == "" {
		tail += `echo
msg_no_connect_plugin
`
		if !market {
			return tail
		}
		return tail + `msg_index_hint "$INDEX"
`
	}
	return tail + `CONNECT_PKG=` + shellQuote(pkg) + `
if command -v dsh >/dev/null 2>&1; then
  DSH_CMD="dsh"
elif command -v npx >/dev/null 2>&1; then
  DSH_CMD="npx --yes @deepseek-ai/dsh"
else
  DSH_CMD=""
fi
if ! command -v pnpm >/dev/null 2>&1; then
  msg_need_pnpm
  exit 1
fi
if [ -z "$DSH_CMD" ]; then
  msg_no_dsh_cmd '` + connectProfile + `' "$CONNECT_PKG"
  exit 1
fi
PROFILES="${DSH_PROFILES:-` + connectProfile + `}"
if [ -z "$(printf '%s' "$PROFILES" | tr -d '[:space:]')" ]; then
  msg_profiles_empty
  exit 1
fi
for profile in $PROFILES; do
  if ! $DSH_CMD plugin --profile "$profile" add "$CONNECT_PKG@latest"; then
    msg_plugin_add_failed "$profile" "$CONNECT_PKG"
    exit 1
  fi
  msg_installed "$profile" "$CONNECT_PKG"
done
echo
msg_done
msg_hub_hint "$BASE"
`
}

// setupEnvPowerShell 是 setup.ps1 里写 .env 的那段；独立出来，Windows 测试可以直接执行它。
func setupEnvPowerShell(txt ps1Text) string {
	return `
  $EnvFile = Join-Path $DshDir '.env'
  if (-not $DeviceLogin) {
    New-Item -ItemType Directory -Force -Path $DshDir | Out-Null
    $lines = @()
    if (Test-Path $EnvFile) { $lines = @(Get-Content $EnvFile -Encoding UTF8 | Where-Object { $_ -notmatch '^` + connectTokenEnv + `=' }) }
    $lines += "` + connectTokenEnv + `=$Token"
    # ` + txt.envBomComment1 + `
    # ` + txt.envBomComment2 + `
    [System.IO.File]::WriteAllLines($EnvFile, [string[]]$lines)
    # ` + txt.envAclComment + `
    icacls $EnvFile /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    Write-Host "` + txt.envTokenWritten + `"
  } else {
    Write-Host "` + txt.envPendingSaved + `"
    if ((Test-Path $EnvFile) -and (Select-String -Path $EnvFile -Pattern '^` + connectTokenEnv + `=' -Quiet)) {
      Write-Host '` + txt.envOldTokenHint + `'
    }
  }
`
}

// connectPowerShell 是 setup.ps1 的后半段。
func (s *Server) connectPowerShell(r *http.Request, txt ps1Text) string {
	pkg, market := s.connectSetup(r)
	tail := setupEnvPowerShell(txt)

	if pkg == "" {
		tail += `  Write-Host ''
  Write-Host '` + txt.noConnectPkg + `'
`
		if market {
			tail += `  Write-Host '` + txt.indexHint + `'
  Write-Host "  $Index"
`
		}
		return tail
	}
	return tail + `  $ConnectPkg = ` + psQuote(pkg) + `
  if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) { Write-Error '` + txt.pnpmMissing + `'; return }
` + connectInstallPowerShell(txt)
}

// connectInstallPowerShell 是 setup.ps1 在各 profile 里装接入插件的那段；独立出来，Windows 测试可以直接执行它。
func connectInstallPowerShell(txt ps1Text) string {
	return `  foreach ($ProfileName in $Profiles) {
    if (Get-Command dsh -ErrorAction SilentlyContinue) { dsh plugin --profile $ProfileName add "$ConnectPkg@latest" }
    else { npx --yes '@deepseek-ai/dsh' plugin --profile $ProfileName add "$ConnectPkg@latest" }
    # ` + txt.nativeExitComment + `
    if ($LASTEXITCODE -ne 0) { Write-Error "` + txt.pluginAddFailed + `"; return }
    Write-Host "` + txt.pluginInstalled + `"
  }
  Write-Host '` + txt.done + `'
  Write-Host "` + txt.autoSync + `"
`
}
