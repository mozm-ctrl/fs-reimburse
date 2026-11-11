#!/bin/sh
# Builds dist/fsr-macos.zip: a universal (Apple Silicon + Intel) fsr binary, the skill and a
# one-step installer, so colleagues need neither Go nor access to this repo.
# Usage: sh package.sh
set -e
cd "$(dirname "$0")"
VERSION=$(git describe --always --dirty 2>/dev/null || echo dev)
OUT=dist/fsr-macos
rm -rf dist && mkdir -p "$OUT"
for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -ldflags "-s -w -X main.version=$VERSION" -o "dist/fsr-$arch" .
done
lipo -create -output "$OUT/fsr" dist/fsr-arm64 dist/fsr-amd64
codesign --force --sign - "$OUT/fsr"
rm dist/fsr-arm64 dist/fsr-amd64
cp SKILL.md "$OUT/"
cat > "$OUT/install.sh" <<'EOF'
#!/bin/sh
# Installs fsr to ~/.local/bin and the skill to ~/.claude/skills/fs-reimburse.
set -e
cd "$(dirname "$0")"
BIN_DIR="$HOME/.local/bin"
SKILL_DIR="$HOME/.claude/skills/fs-reimburse"
mkdir -p "$BIN_DIR" "$SKILL_DIR"
cp fsr "$BIN_DIR/fsr"
chmod 755 "$BIN_DIR/fsr"
# Downloaded files are quarantined; without this macOS refuses to run the unsigned binary.
xattr -d com.apple.quarantine "$BIN_DIR/fsr" 2>/dev/null || true
cp SKILL.md "$SKILL_DIR/SKILL.md"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    RC="$HOME/.zshrc"
    echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$RC"
    echo "已把 ~/.local/bin 加入 PATH（$RC），请重新打开终端"
    ;;
esac
echo "安装完成：fsr $("$BIN_DIR/fsr" version | cut -d' ' -f2)，skill 已装到 Claude Code"
echo "下一步：在终端运行 fsr login 登录报销网站"
EOF
cat > "$OUT/使用说明.txt" <<'EOF'
报销上传 skill（macOS）

安装（一次）：
1. 解压这个压缩包
2. 打开“终端”，输入 sh ，加一个空格，把解压出来的 install.sh 拖进终端窗口，按回车
3. 运行 fsr login，输入报销网站的邮箱和密码（密码不会保存）

使用：
打开 Claude Code，把小票图片拖进去，说“帮我把这张小票按餐饮报销上传”。
采购就说“这张是采购，用途是 xxx”。智能体会先跟你确认，再提交，然后给出记录 ID。

注意：
- 电脑要能访问公司内网（10.1.2.247），在外面请先连 VPN
- 当天要先在报销网站打卡
- 需要先装好 Claude Code
EOF
(cd dist && zip -q -r fsr-macos.zip fsr-macos)
echo "dist/fsr-macos.zip"
