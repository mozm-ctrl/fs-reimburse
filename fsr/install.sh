#!/bin/sh
# Installs the fsr CLI to ~/.local/bin and the skill to ~/.claude/skills/fs-reimburse.
# Usage: sh install.sh [--skills-dir DIR] [--bin-dir DIR]
set -e
cd "$(dirname "$0")"
BIN_DIR="$HOME/.local/bin"
SKILLS_DIR="$HOME/.claude/skills"
while [ $# -gt 0 ]; do
  case "$1" in
    --bin-dir) BIN_DIR="$2"; shift 2 ;;
    --skills-dir) SKILLS_DIR="$2"; shift 2 ;;
    *) echo "unknown option $1" >&2; exit 1 ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "需要 Go 1.26+ 来编译 fsr：https://go.dev/dl/" >&2
  exit 1
fi
mkdir -p "$BIN_DIR" "$SKILLS_DIR/fs-reimburse"
CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(git describe --always --dirty 2>/dev/null || echo dev)" -o "$BIN_DIR/fsr" .
cp SKILL.md "$SKILLS_DIR/fs-reimburse/SKILL.md"
echo "已安装 fsr 到 $BIN_DIR/fsr，skill 到 $SKILLS_DIR/fs-reimburse"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "注意：$BIN_DIR 不在 PATH 里，请把它加到 shell 配置中" ;;
esac
echo "下一步：运行 fsr login 登录报销网站"
