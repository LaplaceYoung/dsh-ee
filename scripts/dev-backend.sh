#!/usr/bin/env sh
# 本地开发：加载 server/.env.local，用 go run 起后端（默认 127.0.0.1:8080）。
# 数据库先起好：cd deploy && docker compose -f compose.yml -f compose.tidb.yml -f compose.dev.yml up -d tidb
set -eu
cd "$(dirname "$0")/../server"
export PATH="/usr/local/go/bin:$PATH"
set -a
. ./.env.local
set +a
exec go run ./cmd/dsh-ee
