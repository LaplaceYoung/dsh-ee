#!/usr/bin/env bash
# 公开演示站：不用 docker、不用 root，一个普通用户就能跑。
# 本机起一个单机 TiDB（unistore）加 dsh-ee，定期把两边的数据全删掉、重新导入示例内容。
#
#   deploy/demo/demo.sh init     生成 $DEMO_HOME/demo.env（主密钥、管理员密码），只需一次
#   deploy/demo/demo.sh start    启动 TiDB 与 dsh-ee（已在跑的跳过）
#   deploy/demo/demo.sh stop     全部停掉
#   deploy/demo/demo.sh reset    停掉、清空数据、重新启动并导入示例内容
#   deploy/demo/demo.sh status   看各进程是否在跑
#   deploy/demo/demo.sh stats    按天看访客点了多少次场景卡片、打开了多少次应用、接入了多少次 DSH
#
# crontab 示例（每 6 小时重置一次，开机自动起）：
#   0 */6 * * * /path/to/deploy/demo/demo.sh reset >> $HOME/dsh-ee-demo/logs/reset.log 2>&1
#   @reboot     /path/to/deploy/demo/demo.sh start >> $HOME/dsh-ee-demo/logs/reset.log 2>&1
#
# 对外访问：dsh-ee 只听本机端口。demo.env 里设了 CF_TUNNEL_CONFIG 时，start / stop 会一起起停
# Cloudflare Tunnel（重置时不动它）；也可以前面放 nginx 反代。见 deploy/demo/README.md。
set -euo pipefail

DEMO_HOME=${DEMO_HOME:-$HOME/dsh-ee-demo}
ENV_FILE=$DEMO_HOME/demo.env
LOGS=$DEMO_HOME/logs
RUN=$DEMO_HOME/run

log() { printf '%s %s\n' "$(date '+%F %T')" "$*"; }
die() { log "错误：$*" >&2; exit 1; }

load_env() {
	[[ -f $ENV_FILE ]] || die "没有 $ENV_FILE，先运行 $0 init"
	set -a
	# shellcheck disable=SC1090
	. "$ENV_FILE"
	set +a
	: "${DSH_EE_BIN:?demo.env 里要写 DSH_EE_BIN（dsh-ee 二进制路径）}"
	: "${TIDB_BIN:?demo.env 里要写 TIDB_BIN（tidb-server 路径）}"
	DEMO_PORT=${DEMO_PORT:-8095}
	TIDB_PORT=${TIDB_PORT:-4100}
	TIDB_STATUS_PORT=${TIDB_STATUS_PORT:-10180}
	TIDB_MEMORY_LIMIT=${TIDB_MEMORY_LIMIT:-1536MB}
	CF_TUNNEL_CONFIG=${CF_TUNNEL_CONFIG:-}
	export DSH_EE_ADDR=127.0.0.1:$DEMO_PORT
	export DSH_EE_DSN="root:@tcp(127.0.0.1:$TIDB_PORT)/dsh_ee"
	export DSH_EE_DATA_DIR=$DEMO_HOME/data
	export DSH_EE_DEMO_MODE=1
	export DSH_EE_DB_WAIT=${DSH_EE_DB_WAIT:-2m}
	# 访问统计放在 logs 下、不在轮换的名单里：重置清的是数据库和数据目录，这个文件一直留着
	export DSH_EE_DEMO_STATS_FILE=${DSH_EE_DEMO_STATS_FILE:-$LOGS/stats.jsonl}
}

cmd_init() {
	mkdir -p "$DEMO_HOME" "$LOGS" "$RUN"
	[[ -f $ENV_FILE ]] && die "$ENV_FILE 已存在，不覆盖"
	local tidb dsh
	tidb=$(ls -d "$HOME"/.tiup/components/tidb/*/tidb-server 2>/dev/null | sort -V | tail -1 || true)
	dsh=$(cd "$(dirname "$0")/../.." && pwd)/dist/dsh-ee
	umask 077
	cat > "$ENV_FILE" <<ENV
# 演示站配置；demo.sh 每次启动都会读这个文件。
DSH_EE_BIN=$dsh
TIDB_BIN=${tidb:-/path/to/tidb-server}
DEMO_PORT=8095
TIDB_PORT=4100
TIDB_STATUS_PORT=10180
# TiDB 内存上限（unistore 不设上限会越吃越多）
TIDB_MEMORY_LIMIT=1536MB

# 对外地址（nginx 反代后的 https 地址）；填了 https 会自动给 cookie 加 Secure
DSH_EE_PUBLIC_URL=
DSH_EE_COOKIE_SECURE=
DSH_EE_SECRET_KEY=$(openssl rand -base64 32)
DSH_EE_BOOTSTRAP_ADMIN=admin
# 这个密码会显示在登录页上，所有访客共用
DSH_EE_BOOTSTRAP_ADMIN_PASSWORD=demo-$(openssl rand -hex 3)
# Cloudflare Tunnel 的配置文件（cloudflared tunnel create 之后自己写）；不用就留空
CF_TUNNEL_CONFIG=

# 给访客看的重置周期，和 crontab 保持一致
DSH_EE_DEMO_RESET="每 6 小时"
# 实际的重置间隔（对应 crontab 的 0 */6 * * *），登录页据此显示「下次重置：还剩多久」；留空不显示
DSH_EE_DEMO_RESET_EVERY=6h
# 首页介绍视频所在目录（intro-zh.mp4、intro-en.mp4，见 promo/README.md）；留空不显示
DSH_EE_DEMO_VIDEO_DIR=
ENV
	log "已生成 $ENV_FILE，检查一下 DSH_EE_BIN / TIDB_BIN / DSH_EE_PUBLIC_URL 再 $0 reset"
}

pid_alive() { [[ -f $RUN/$1.pid ]] && kill -0 "$(cat "$RUN/$1.pid")" 2>/dev/null; }

wait_port() {
	local port=$1 i
	for i in $(seq 1 60); do
		(echo > "/dev/tcp/127.0.0.1/$port") 2>/dev/null && return 0
		sleep 1
	done
	return 1
}

start_tidb() {
	if pid_alive tidb; then log "TiDB 已在运行"; return; fi
	mkdir -p "$DEMO_HOME/tidb" "$LOGS" "$RUN"
	# 直接在本 shell 里后台启动，$! 才是 tidb-server 自己；9>&- 不让它拿着 reset 的锁
	nohup "$TIDB_BIN" -store unistore -path "$DEMO_HOME/tidb/data" \
		-host 127.0.0.1 -P "$TIDB_PORT" -status "$TIDB_STATUS_PORT" \
		-log-file "$LOGS/tidb.log" -log-slow-query "$LOGS/tidb-slow.log" \
		< /dev/null > "$LOGS/tidb.out" 2>&1 9>&- &
	echo $! > "$RUN/tidb.pid"
	wait_port "$TIDB_PORT" || die "TiDB 没在 60 秒内起来，看 $LOGS/tidb.log"
	log "TiDB 已启动（127.0.0.1:$TIDB_PORT）"
	# 内存上限是系统变量，只能连上以后设；没有 mysql 客户端就跳过，靠定期重置兜底
	local mysql
	mysql=$(command -v mysql || ls /www/server/mysql/bin/mysql 2>/dev/null || true)
	if [[ -n $mysql ]]; then
		"$mysql" -h 127.0.0.1 -P "$TIDB_PORT" -u root -e "SET GLOBAL tidb_server_memory_limit='$TIDB_MEMORY_LIMIT'" \
			&& log "TiDB 内存上限 $TIDB_MEMORY_LIMIT" || log "设 TiDB 内存上限失败，忽略"
	fi
}

start_dsh() {
	if pid_alive dsh-ee; then log "dsh-ee 已在运行"; return; fi
	[[ -x $DSH_EE_BIN ]] || die "找不到 $DSH_EE_BIN，先在仓库根目录 make build"
	mkdir -p "$DSH_EE_DATA_DIR"
	nohup "$DSH_EE_BIN" < /dev/null >> "$LOGS/dsh-ee.log" 2>&1 9>&- &
	echo $! > "$RUN/dsh-ee.pid"
	local i
	for i in $(seq 1 120); do
		curl -fsS "http://127.0.0.1:$DEMO_PORT/healthz" >/dev/null 2>&1 && { log "dsh-ee 已启动（127.0.0.1:$DEMO_PORT）"; return; }
		pid_alive dsh-ee || die "dsh-ee 启动失败，看 $LOGS/dsh-ee.log"
		sleep 1
	done
	die "dsh-ee 没在 2 分钟内就绪，看 $LOGS/dsh-ee.log"
}

start_tunnel() {
	[[ -n $CF_TUNNEL_CONFIG ]] || return 0
	if pid_alive cloudflared; then log "cloudflared 已在运行"; return; fi
	command -v cloudflared >/dev/null || die "设了 CF_TUNNEL_CONFIG 但找不到 cloudflared"
	nohup cloudflared --no-autoupdate tunnel --config "$CF_TUNNEL_CONFIG" run \
		< /dev/null >> "$LOGS/cloudflared.log" 2>&1 9>&- &
	echo $! > "$RUN/cloudflared.pid"
	sleep 3
	pid_alive cloudflared || die "cloudflared 启动失败，看 $LOGS/cloudflared.log"
	log "cloudflared 已启动"
}

stop_one() {
	local name=$1 pid i
	pid_alive "$name" || { rm -f "$RUN/$name.pid"; return; }
	pid=$(cat "$RUN/$name.pid")
	kill "$pid"
	for i in $(seq 1 30); do kill -0 "$pid" 2>/dev/null || break; sleep 1; done
	kill -0 "$pid" 2>/dev/null && kill -9 "$pid"
	rm -f "$RUN/$name.pid"
	log "$name 已停止"
}

import_demo() {
	local jar base=http://127.0.0.1:$DEMO_PORT
	jar=$(mktemp)
	curl -fsS -c "$jar" -H 'Content-Type: application/json' \
		-d "$(printf '{"username":"%s","password":"%s"}' "${DSH_EE_BOOTSTRAP_ADMIN:-admin}" "$DSH_EE_BOOTSTRAP_ADMIN_PASSWORD")" \
		"$base/api/auth/login" >/dev/null || { rm -f "$jar"; die "管理员登录失败"; }
	# 示例公司和小团队两个场景都导入，登录页上让访客自己挑
	local sc
	for sc in xinghe novel; do
		if ! curl -fsS -b "$jar" -X POST "$base/api/admin/demo?scenario=$sc" > "$LOGS/last-import-$sc.json"; then
			rm -f "$jar"
			die "导入示例场景 $sc 失败，看 $LOGS/last-import-$sc.json"
		fi
	done
	rm -f "$jar"
	log "示例内容已导入"
}

cmd_reset() {
	# 同一时间只跑一个 reset（cron 和手动撞上时）
	exec 9> "$RUN/reset.lock"
	flock -n 9 || die "另一个 reset 正在进行"
	log "开始重置"
	stop_one dsh-ee
	stop_one tidb
	rm -rf "$DEMO_HOME/tidb/data" "$DSH_EE_DATA_DIR"
	# 日志只留最近一份，免得越积越多
	for f in tidb.log tidb-slow.log dsh-ee.log; do
		[[ -f $LOGS/$f ]] && mv -f "$LOGS/$f" "$LOGS/$f.1"
	done
	rm -rf "$LOGS"/tidb-*.log "$LOGS/oom_record" 2>/dev/null || true
	start_tidb
	start_dsh
	import_demo
	start_tunnel
	log "重置完成"
}

cmd_status() {
	pid_alive tidb && echo "TiDB    运行中（pid $(cat "$RUN/tidb.pid")）" || echo "TiDB    未运行"
	pid_alive dsh-ee && echo "dsh-ee  运行中（pid $(cat "$RUN/dsh-ee.pid")）" || echo "dsh-ee  未运行"
	if [[ -n $CF_TUNNEL_CONFIG ]]; then
		pid_alive cloudflared && echo "tunnel  运行中（pid $(cat "$RUN/cloudflared.pid")）" || echo "tunnel  未运行"
	fi
}

cmd_stats() {
	[[ -s $DSH_EE_DEMO_STATS_FILE ]] || { echo "还没有统计（$DSH_EE_DEMO_STATS_FILE）"; return; }
	# 每行 {"event":"persona","t":"2026-11-11T08:00:00Z","what":"zhou.hang"}：按天、按动作数次数
	sed -E 's/.*"event":"([^"]*)".*"t":"([0-9-]{10}).*/\2 \1/' "$DSH_EE_DEMO_STATS_FILE" | sort | uniq -c
}

case ${1:-} in
init) cmd_init ;;
start) load_env; mkdir -p "$RUN" "$LOGS"; start_tidb; start_dsh; start_tunnel ;;
stop) load_env; stop_one cloudflared; stop_one dsh-ee; stop_one tidb ;;
reset) load_env; mkdir -p "$RUN" "$LOGS"; cmd_reset ;;
status) load_env; cmd_status ;;
stats) load_env; cmd_stats ;;
*) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
