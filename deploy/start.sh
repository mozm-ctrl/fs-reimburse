#!/bin/zsh
# Loads credentials from ~/fs-reimburse/.env (chmod 600) and starts the forwarder (Go build).
# Source: ~/fs-reimburse-go. Rollback to the Node version: cp start.node.sh start.sh && ./fsctl.sh restart
cd "$(dirname "$0")"
if [[ ! -f .env ]]; then echo "请先填写 $(pwd)/.env"; exit 1; fi
set -a; source ./.env; set +a
exec ./fs-reimburse serve
