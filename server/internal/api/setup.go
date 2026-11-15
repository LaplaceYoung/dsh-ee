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

func (s *Server) handleSetupPs1(w http.ResponseWriter, r *http.Request) {
	base, registry, authKey, scopes, err := s.setupVars(r)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	var sc []string
	for _, x := range scopes {
		sc = append(sc, psQuote(x))
	}
	script := `# dsh-ee 接入脚本：默认通过浏览器设备登录；-Token 保留 CI 与旧流程。
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
    Write-Error '当前环境不能进行浏览器设备登录，请使用 -Token <令牌> 或设置 DSH_EE_TOKEN。'
    return
  }
  if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    Write-Error '没有找到 npm：请先安装 Node.js。'
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
    catch { Write-Error "设备登录发起失败：$_"; return }
    Write-Host '请在浏览器中确认登录（已尝试自动打开）：'
    Write-Host "  $($flow.verification_uri_complete)"
    Write-Host "确认码：$($flow.user_code)"
    Write-Host '等待确认…（Ctrl-C 取消）'
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
        Write-Error "设备登录失败：$($pollError.error)"
        return
      }
      if ($login.access_token) { break }
    }
    if (-not $login.access_token) { Write-Error '设备登录超时，请重新运行 setup'; return }
    New-Item -ItemType Directory -Force -Path $PendingDir | Out-Null
    [System.IO.File]::WriteAllText($Pending, $login.access_token + [Environment]::NewLine)
    icacls $PendingDir /inheritance:r /grant:r "${env:USERNAME}:(OI)(CI)(F)" | Out-Null
    icacls $Pending /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    $Token = $login.access_token
    Write-Host "✔ 已登录 $($login.user.username)（$($login.user.department)）"
  }
  if ($Token -notmatch '^[A-Za-z0-9._~-]+$') { Write-Error '令牌格式不对'; return }
  try { $me = Invoke-RestMethod -Uri "$Base/api/dsh/whoami" -Headers @{ Authorization = "Bearer $Token" } }
  catch { Write-Error '令牌无效或连不上 dsh-ee'; return }
  Write-Host "令牌有效，当前账号：$($me.username)"
  $Scopes = @(` + strings.Join(sc, ", ") + `)
  $rawProfiles = if ($env:DSH_PROFILES) { $env:DSH_PROFILES } else { '` + connectProfile + `' }
  $Profiles = @($rawProfiles -split '\s+' | Where-Object { $_ })
  if ($Profiles.Count -eq 0) { Write-Error 'DSH_PROFILES 不能为空'; return }
  foreach ($ProfileName in $Profiles) {
    # 只写 DSH profile 目录的 .npmrc（DSH 跑 pnpm 时的工作目录），不动 ~/.npmrc
    $ProfileDir = Join-Path (Join-Path $DshDir 'profiles') $ProfileName
    $NpmrcFile = Join-Path $ProfileDir '.npmrc'
    New-Item -ItemType Directory -Force -Path $ProfileDir | Out-Null
    $drop = @('registry', $AuthKey) + ($Scopes | ForEach-Object { "${_}:registry" })
    $lines = @()
    if (Test-Path $NpmrcFile) { $lines = @(Get-Content $NpmrcFile -Encoding UTF8 | Where-Object { $drop -notcontains ($_ -split '=', 2)[0] }) }
    $lines += "registry=$Registry"
    foreach ($scope in $Scopes) { $lines += "${scope}:registry=$Registry" }
    $lines += "$AuthKey=$Token"
    # .NET 默认写无 BOM 的 UTF-8；Set-Content -Encoding utf8 在 PowerShell 5 会加 BOM
    [System.IO.File]::WriteAllLines($NpmrcFile, [string[]]$lines)
    icacls $NpmrcFile /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    Write-Host "已写入 $NpmrcFile（只对 DSH 生效，不改 ~/.npmrc）"
  }
  if ($env:DSH_EE_GLOBAL_NPMRC -eq '1') {
    npm config set registry $Registry
    npm config set $AuthKey $Token
    foreach ($scope in $Scopes) { npm config set "${scope}:registry" $Registry }
    Write-Host '已按 DSH_EE_GLOBAL_NPMRC=1 同时写入 ~/.npmrc'
  }
` + s.connectPowerShell(r) + `}
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
  $DSH_CMD plugin --profile "$profile" add "$CONNECT_PKG@latest"
  msg_installed "$profile" "$CONNECT_PKG"
done
echo
msg_done
msg_hub_hint "$BASE"
`
}

// setupEnvPowerShell 是 setup.ps1 里写 .env 的那段；独立出来，Windows 测试可以直接执行它。
func setupEnvPowerShell() string {
	return `
  $EnvFile = Join-Path $DshDir '.env'
  if (-not $DeviceLogin) {
    New-Item -ItemType Directory -Force -Path $DshDir | Out-Null
    $lines = @()
    if (Test-Path $EnvFile) { $lines = @(Get-Content $EnvFile -Encoding UTF8 | Where-Object { $_ -notmatch '^` + connectTokenEnv + `=' }) }
    $lines += "` + connectTokenEnv + `=$Token"
    # .NET 默认写无 BOM 的 UTF-8；Set-Content -Encoding utf8 在 PowerShell 5 会加 BOM，
    # Node 的 process.loadEnvFile 会把 BOM 当成变量名的一部分，DSHEE_TOKEN 就读不到。
    [System.IO.File]::WriteAllLines($EnvFile, [string[]]$lines)
    # .env 里也是接入令牌，权限和 .npmrc / token.pending 保持一致
    icacls $EnvFile /inheritance:r /grant:r "${env:USERNAME}:(F)" | Out-Null
    Write-Host "已把接入令牌写进 $EnvFile（` + connectTokenEnv + `）"
  } else {
    Write-Host "令牌已保存到 $Pending，DSH 启动后会导入凭据库并删除此文件"
    if ((Test-Path $EnvFile) -and (Select-String -Path $EnvFile -Pattern '^` + connectTokenEnv + `=' -Quiet)) {
      Write-Host '提示：已保留 .env 中的旧令牌；凭据库优先，如不再使用请手动删除该行。'
    }
  }
`
}

// connectPowerShell 是 setup.ps1 的后半段。
func (s *Server) connectPowerShell(r *http.Request) string {
	pkg, market := s.connectSetup(r)
	tail := setupEnvPowerShell()

	if pkg == "" {
		tail += `  Write-Host ''
  Write-Host '管理员还没有发布 DSH 接入插件；发布后重新运行本脚本即可接入企业技能与系统。'
`
		if market {
			tail += `  Write-Host '现在可以先在 DSH 插件页 → 发现 → 添加索引源，填入'
  Write-Host "  $Index"
`
		}
		return tail
	}
	return tail + `  $ConnectPkg = ` + psQuote(pkg) + `
  if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) { Write-Error '安装 DSH 插件需要 pnpm：先运行 npm install -g pnpm，再重新运行本脚本'; return }
  foreach ($ProfileName in $Profiles) {
    if (Get-Command dsh -ErrorAction SilentlyContinue) { dsh plugin --profile $ProfileName add "$ConnectPkg@latest" }
    else { npx --yes '@deepseek-ai/dsh' plugin --profile $ProfileName add "$ConnectPkg@latest" }
    Write-Host "已在 DSH 的 $ProfileName profile 里装好 $ConnectPkg"
  }
  Write-Host '完成。重启 DSH（dsh web）后，企业技能、提示词、微应用与企业系统就会出现。'
  Write-Host "以后在 $Base/hub 启用新的能力，DSH 会自动同步。"
`
}
