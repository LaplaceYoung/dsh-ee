package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// CLI 的输出语言（P5 五语化：CLI 只做 zh/en；服务端与网页的 i18n 是另一套，见 cli_package.go 里
// scanText 的说明）。
//
// 优先级：显式 --lang > 环境变量 DSH_EE_LANG > 系统 LC_ALL / LANG（zh* → zh，其它 → en）> 默认 zh。
// runCLI 在分发给子命令之前统一解析一次，存进包级 cliLang；所有输出函数（cliText / tr）都读它。
// --lang 可以写在任意子命令的参数里，解析完就摘掉，子命令自己的参数解析不需要认识它。
const (
	langZH = "zh"
	langEN = "en"
)

// cliLang 是本次进程的输出语言，默认中文（既有测试与既有脚本都按中文输出）。
var cliLang = langZH

// cliText 是一段固定的双语文案；String() 按 cliLang 选一边。
// 实现了 fmt.Stringer，所以 fmt.Fprint(w, cliUsage) 会直接打印当前语言的那份。
type cliText struct{ zh, en string }

func (t cliText) String() string {
	if cliLang == langEN {
		return t.en
	}
	return t.zh
}

// tr 返回当前语言下的一段内联文案。成块的固定文案（help/usage）用 cliText，一次性的提示与
// 错误用 tr，两种都能用 grep 找到中文原文与英文译文。
func tr(zh, en string) string {
	if cliLang == langEN {
		return en
	}
	return zh
}

// joinText 按当前语言连接一组词：中文用「、」，英文用逗号加空格。
func joinText(xs []string) string {
	return strings.Join(xs, tr("、", ", "))
}

// takeCLILang 从参数里摘出全局 --lang（--lang en / --lang=en，可出现在任意位置），
// 解析出本次语言存入 cliLang，返回去掉 --lang 之后的参数。
func takeCLILang(args []string) ([]string, error) {
	rest := make([]string, 0, len(args))
	explicit := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--lang":
			if i+1 >= len(args) {
				cliLang = langFromEnv()
				return nil, errors.New(tr("--lang 后面要跟一个值（zh 或 en）", "--lang requires a value (zh or en)"))
			}
			i++
			explicit = args[i]
		case strings.HasPrefix(a, "--lang="):
			explicit = strings.TrimPrefix(a, "--lang=")
		default:
			rest = append(rest, a)
		}
	}
	if explicit != "" {
		if lang, ok := parseLang(explicit); ok {
			cliLang = lang
			return rest, nil
		}
		cliLang = langFromEnv()
		return nil, fmt.Errorf(tr("--lang 只支持 zh / en，给的是 %q", "--lang only supports zh / en, got %q"), explicit)
	}
	cliLang = langFromEnv()
	return rest, nil
}

// parseLang 认 zh / en 以及 zh-CN、en_US.UTF-8 这类带地区的写法（只看前缀）。
func parseLang(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case strings.HasPrefix(v, "zh"):
		return langZH, true
	case strings.HasPrefix(v, "en"):
		return langEN, true
	}
	return "", false
}

// langFromEnv 读环境变量：DSH_EE_LANG 优先，其次是 LC_ALL、LANG。
// LC_ALL/LANG 是 zh* 才用中文，其它语言（如 en_US、fr_FR）统一退到英文；
// C / POSIX 这类「没有语言偏好」的 locale 不算选择，继续往下看，最后默认中文。
func langFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("DSH_EE_LANG")); v != "" {
		if lang, ok := parseLang(v); ok {
			return lang
		}
	}
	for _, key := range []string{"LC_ALL", "LANG"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" || isNeutralLocale(v) {
			continue
		}
		if lang, ok := parseLang(v); ok {
			return lang
		}
		return langEN
	}
	return langZH
}

// isNeutralLocale 判断 C / POSIX / C.UTF-8 这类不表达语言偏好的 locale。
func isNeutralLocale(v string) bool {
	return v == "C" || v == "POSIX" || strings.HasPrefix(v, "C.")
}

// 固定文案集合：help / usage。中文原文紧挨着各自的 const 定义（cliUsageZH 等，见文件末尾），
// 这里只放英文译文并按当前语言组合，改动任何一份都很容易 grep 到。
var (
	cliUsage = cliText{zh: cliUsageZH, en: `Usage:
  dsh-ee                                 start the server
  dsh-ee version                         print the version
  dsh-ee extension init <dir> --id author/name [--kind declarative|service]
                                         create an extension (with AGENTS.md); edit the copy from there
  dsh-ee extension check <dir> [--json]  check an extension: manifest, OpenAPI doc, read-only flags, credential style
  dsh-ee extension test <dir|url> [--run "start command"] [--credential value] [--config JSON] [--write] [--json]
                                         run a service extension and test it against the protocol item by item
  dsh-ee extension dev [--listen addr]   start a fake dsh-ee (no database) for extension development
  dsh-ee extension call <url> <tool> [--as user] [--args JSON] [--credential value] [--config JSON]
                                         call one extension tool as a given user
  dsh-ee extension pack <dir> [-o file]  pack a declarative extension into a zip and print the checksum (signatures included)
  dsh-ee extension report <dir|url>      generate a hand-off report for admins (Markdown)
  dsh-ee extension keygen <private-key-file> [--public public-key-file]
                                         generate an Ed25519 key for signing; the private key stays on this machine
  dsh-ee extension sign <dir> --key <private-key-file> [-o file]
                                         sign the manifest (and the OpenAPI doc for declarative extensions), write signature.json
  dsh-ee extension verify <dir> [--key public-key-file]
                                         verify the signature; exit code 1 when unsigned or invalid
  dsh-ee directory sign <file> --key <private-key-file> [-o file]
                                         sign a whole enterprise directory (dsh-ee-directory.json); writes in place by default
  dsh-ee directory verify <file> [--key public-key-file]
                                         verify the directory signature and entries; exit code 1 when unsigned or invalid
  dsh-ee config export|plan|test|apply|explain|simulate …
                                         manage permission config (roles, fields, access rules) as files; see dsh-ee config help
  dsh-ee package check <file.tgz> [--json]
                                         hand an npm package to the server for a pre-publish check; nothing is published
  dsh-ee doctor [--json]                 read-only diagnosis of the publish path: server, token scopes, npmrc, Node/dsh
  dsh-ee admin recover                   create a recovery account valid for 1 hour on the server (for lockouts)
  dsh-ee schema list [--json]            list schemas
  dsh-ee schema get <name>               print one schema (manifest: manifest; errors: check list)

extension check exits 1 on errors. Add --json for machine-readable results (for programs and agents):
each entry has code, severity, where (file and position), message, fix (the concrete change).
For package check and doctor the address/token can also come from DSH_EE_URL, DSH_EE_TOKEN, .npmrc, or $DSH_HOME/dsh-ee/token.

Signing: sign signs "author/name@version:digest", where the digest is the sha256 printed by check (of the manifest and
the OpenAPI doc); the Ed25519 detached signature is stored in signature.json in the extension directory. Trusted public
keys can be given with --key or DSH_EE_EXTENSION_TRUSTED_KEYS; the dsh-ee server uses the same env var names, and with
DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1 extensions that are unsigned or fail verification cannot be installed (by default
this is only shown in the install plan, without blocking).

Enterprise directory (format 2, docs/plugin-signing.md §8): directory sign signs the canonical JSON of the directory
minus signature; directory verify checks it with --key or DSH_EE_EXTENSION_TRUSTED_KEYS; tarball hosts on the intranet
must be listed in DSH_EE_DIRECTORY_TARBALL_HOSTS.
`}

	doctorUsage = cliText{zh: doctorUsageZH, en: `Usage: dsh-ee doctor [--server URL] [--token TOKEN] [--json]

Read-only diagnosis of the publish path; no configuration is changed:
  where the server URL and token are read from (flags, env vars, .npmrc, $DSH_HOME/dsh-ee/token)
  server reachability and version (GET /healthz)
  whether the token is valid and can call the publish precheck (needs registry:publish or agent scope)
  whether the enterprise registry and token in .npmrc match the current login
  whether the enterprise npm registry is reachable
  local Node and dsh versions

Connection parameters are the same as dsh-ee package check; --json prints machine-readable results.
Exit codes: 0 no failures (warnings allowed), 1 failures, 2 usage error.
`}

	packageUsage = cliText{zh: packageUsageZH, en: `Usage: dsh-ee package check <file.tgz> [--server URL] [--token TOKEN] [--json]

Hand an npm pack .tgz to dsh-ee for a pre-publish check: automatic checks and publish conflicts are both reviewed,
without writing the database or claiming a version. Pack the file locally first; this command only uploads and shows results.

Connection parameters (env vars also work):
  --server URL    or DSH_EE_URL     dsh-ee address, e.g. https://ee.acme.com
  --token TOKEN   or DSH_EE_TOKEN   token with registry:publish (or agent) scope

When no address or token is given, .npmrc (NPM_CONFIG_USERCONFIG) and $DSH_HOME/dsh-ee/token are also read;
if setup.sh wrote either one, no extra configuration is needed.

Exit codes:
  0  precheck passed: no publish conflicts and no automatic check failures
  1  blocking problems found: package rejected, publish conflicts, or automatic check failures
  2  usage or local config problem: arguments, file, server address, token
  3  server call failed: network error, invalid or unauthorized token, or server error
`}

	configUsage = cliText{zh: configUsageZH, en: `Usage: dsh-ee config <command> [args]

  export [--json]                         export the current permission config (roles, admin constraints, fields, rules)
  plan <file>                             show what this config would change and who it affects (not applied)
  test [<file>] --tests <test-file>       run access tests; use the current config when no config file is given
  apply <file> [--note note]              submit a change request for an admin to approve on the web page
  changes (or status)                     recent change requests and their status
  explain --user USER --doc DOC-ID [--config FILE]
                                          whether someone can see a document, and which checks were involved
  simulate --users a,b (--kb KB | --docs 1,2) [--config FILE]
                                          a table of who can see what

Connection parameters (env vars also work):
  --server URL    or DSH_EE_URL     dsh-ee address, e.g. https://ee.acme.com
  --token TOKEN   or DSH_EE_TOKEN   token with config scope (create one in "My tokens" on the web page)

Every command accepts --json to output raw results. plan with problems and test with failing cases exit 1.
`}

	adminUsage = cliText{zh: adminUsageZH, en: `Usage: dsh-ee admin recover

  recover   create a recovery account valid for 1 hour on the server (with all admin permissions, independent of the
            super admin). Use it when locked out; run on a server that can reach the database, with the same env vars
            as the service. A record is left in the audit log.
`}

	directoryUsage = cliText{zh: directoryUsageZH, en: `Usage:
  dsh-ee directory sign <file> --key <private-key-file> [-o file] [--tarball-hosts host,...] [--json]
                         sign a whole enterprise directory (dsh-ee-directory.json); writes in place by default
  dsh-ee directory verify <file> [--key public-key-file] [--tarball-hosts host,...] [--json]
                         verify the directory signature and entries; exit code 1 when unsigned or invalid

The directory format is described in docs/plugin-signing.md §8 (format 2). It contains plugins (npm packages) and
extension entries; tarball sources must be https URLs; intranet object storage hosts must be listed in
DSH_EE_DIRECTORY_TARBALL_HOSTS (or --tarball-hosts). Trusted signing public keys are given as a file with --key, or via
DSH_EE_EXTENSION_TRUSTED_KEYS (same name as the server; directory signing reuses the same trust root).
`}
)

// 中文原文（默认语言）：从各文件集中到这里，与上面的英文译文成对。
const cliUsageZH = `用法：
  dsh-ee                                 启动服务
  dsh-ee version                         打印版本
  dsh-ee extension init <目录> --id 作者/名字 [--kind declarative|service]
                                         新建一个扩展（带 AGENTS.md），复制后照着改
  dsh-ee extension check <目录> [--json]  检查一个扩展：清单、接口文档、只读标记、凭据写法
  dsh-ee extension test <目录|地址> [--run "启动命令"] [--credential 值] [--config JSON] [--write] [--json]
                                         把服务扩展跑起来按协议逐项测；全部通过才算合格
  dsh-ee extension dev [--listen 地址]    起一个模拟的 dsh-ee（不要数据库），写扩展时用
  dsh-ee extension call <地址> <工具> [--as 员工] [--args JSON] [--credential 值] [--config JSON]
                                         以某位员工的身份调一次扩展的工具
  dsh-ee extension pack <目录> [-o 文件]   把声明式扩展打成 zip，算出校验和（带签名的连签名一起打）
  dsh-ee extension report <目录|地址>      生成给管理员看的交付说明（Markdown）
  dsh-ee extension keygen <私钥文件> [--public 公钥文件]
                                         生成签名用的 Ed25519 密钥；私钥只留在本机
  dsh-ee extension sign <目录> --key <私钥文件> [-o 文件]
                                         给清单（声明式连同接口文档）签名，写 signature.json
  dsh-ee extension verify <目录> [--key 公钥文件]
                                         核对签名；没有签名或验不过时退出码为 1
  dsh-ee directory sign <文件> --key <私钥文件> [-o 文件]
                                         给企业目录（dsh-ee-directory.json）整体签名；默认原地写回
  dsh-ee directory verify <文件> [--key 公钥文件]
                                         核对目录签名和条目；没有签名或验不过时退出码为 1
  dsh-ee config export|plan|test|apply|explain|simulate …
                                         把权限配置（角色、字段、访问规则）当文件来改，见 dsh-ee config help
  dsh-ee package check <文件.tgz> [--json]
                                         把 npm 包交给服务端做发布前预检，不发布、不占版本号
  dsh-ee doctor [--json]                 只读诊断发布链路：服务连通、令牌范围、npmrc、Node/dsh 版本
  dsh-ee admin recover                   在服务器上生成一个 1 小时有效的恢复账号（把自己锁在外面时用）
  dsh-ee schema list [--json]            列出规格
  dsh-ee schema get <名字>                打印一份规格（manifest：清单；errors：检查项）

extension check 有错误时退出码为 1。加 --json 输出给程序和 Agent 看的结果：
每一条都有 code、severity、where（文件和位置）、message、fix（具体改法）。
package check 和 doctor 的地址/令牌也可以用 DSH_EE_URL、DSH_EE_TOKEN，或读 .npmrc 与 $DSH_HOME/dsh-ee/token。

签名：sign 签的是「作者/名字@版本:摘要」，摘要就是 check 打印的 sha256（清单和接口文档的 sha256），
Ed25519 detached 签名存在扩展目录的 signature.json 里。受信任的公钥文件可以用 --key 给，也可以用
DSH_EE_EXTENSION_TRUSTED_KEYS；dsh-ee 服务端用同名环境变量，DSH_EE_EXTENSION_REQUIRE_SIGNATURE=1 时
没签名或验不过的扩展不让安装（默认只在安装计划里提示，不阻断）。

企业目录（format 2，docs/plugin-signing.md §8）：directory sign 签的是目录里除 signature 之外的
canonical JSON，directory verify 用 --key 或 DSH_EE_EXTENSION_TRUSTED_KEYS 核对；tarball 来源的
内网主机要写进 DSH_EE_DIRECTORY_TARBALL_HOSTS 白名单。
`

const doctorUsageZH = `用法：dsh-ee doctor [--server 地址] [--token 令牌] [--json]

只读诊断发布链路，不改任何配置：
  服务地址与令牌从哪读到（参数、环境变量、.npmrc、$DSH_HOME/dsh-ee/token）
  服务连通与版本（GET /healthz）
  令牌是否有效、能不能调发布预检（需要 registry:publish 或 agent 范围）
  .npmrc 的企业源与令牌是否和当前登录一致
  企业 npm 源是否可达
  本机 Node 与 dsh 的版本

连接参数与 dsh-ee package check 相同；--json 输出机器可读的结果。
退出码：0 没有失败项（可以有提醒），1 有失败项，2 用法错误。
`

const packageUsageZH = `用法：dsh-ee package check <文件.tgz> [--server 地址] [--token 令牌] [--json]

把 npm pack 生成的 .tgz 交给 dsh-ee 做发布前预检：自动检查、发布冲突都看一遍，
不落库、不占版本号。文件要先在本机 pack 好，命令只上传和展示结果。

连接参数（也可以用环境变量）：
  --server 地址   或 DSH_EE_URL     dsh-ee 的地址，例如 https://ee.acme.com
  --token 令牌    或 DSH_EE_TOKEN   带 registry:publish（或 agent）范围的令牌

没给地址或令牌时，会再读 .npmrc（NPM_CONFIG_USERCONFIG）与 $DSH_HOME/dsh-ee/token，
setup.sh 写过其中任意一处就不用重复配置。

退出码：
  0  预检通过：没有发布冲突，也没有自动检查失败项
  1  预检发现阻塞问题：包不被接受、有发布冲突或自动检查失败项
  2  用法或本地配置不对：参数、文件、服务地址、令牌
  3  调用服务失败：网络错误、令牌无效或没有权限、服务端错误
`

const configUsageZH = `用法：dsh-ee config <命令> [参数]

  export [--json]                         导出现在的权限配置（角色、管理约束、字段、规则）
  plan <文件>                              看这份配置生效后会变什么、影响谁（不生效）
  test [<文件>] --tests <测试文件>          跑访问测试；不给配置文件就用现在的配置
  apply <文件> [--note 说明]                提交成变更请求，等管理员在网页上批准
  changes（或 status）                     最近的变更请求和它们的状态
  explain --user 用户 --doc 文档id [--config 文件]
                                          某人能不能看某篇文档、经过了哪些判断
  simulate --users a,b (--kb 知识库 | --docs 1,2) [--config 文件]
                                          一张「谁能看什么」的表

连接参数（也可以用环境变量）：
  --server 地址   或 DSH_EE_URL     dsh-ee 的地址，例如 https://ee.acme.com
  --token 令牌    或 DSH_EE_TOKEN   带 config 范围的令牌（网页「我的令牌」里建）

所有命令都可以加 --json 输出原始结果。plan 有问题、test 有不通过的用例时退出码为 1。
`

const adminUsageZH = `用法：dsh-ee admin recover

  recover   在服务器上生成一个 1 小时有效的恢复账号（有全部管理权限点，不靠超级管理员）。
            把自己锁在外面时用；要在能连到数据库的服务器上执行，读和服务一样的环境变量。审计里留记录。
`

const directoryUsageZH = `用法：
  dsh-ee directory sign <文件> --key <私钥文件> [-o 文件] [--tarball-hosts 主机,...] [--json]
                         给企业目录（dsh-ee-directory.json）整体签名；默认原地写回
  dsh-ee directory verify <文件> [--key 公钥文件] [--tarball-hosts 主机,...] [--json]
                         核对目录签名与条目；没有签名或验不过时退出码为 1

目录格式见 docs/plugin-signing.md §8（format 2）。目录里包含插件（npm 包）和扩展条目，
tarball 来源必须是 https 地址；内网对象存储要把主机写进 DSH_EE_DIRECTORY_TARBALL_HOSTS
（或 --tarball-hosts）白名单。受信任的签名公钥用 --key 给文件，或设
DSH_EE_EXTENSION_TRUSTED_KEYS（和服务端同名；目录签名复用同一套信任根）。
`
