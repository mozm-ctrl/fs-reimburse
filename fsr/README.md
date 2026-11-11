# fsr：让智能体帮你上传报销

`fsr` 是一个命令行工具，加上一份 skill 说明（`SKILL.md`），装好后就可以让自己的 AI 智能体（Claude Code、Multica 等）把小票提交到公司报销网站（nodelink-intranet，`http://10.1.2.247`）。

- 在自己的电脑上运行，不依赖 fs 助手服务，也不需要一直开着某台 Mac
- 只在本机保存登录 token（`~/.config/fs-reimburse/auth.json`，权限 600），**不保存密码**
- 纯 Go 实现，图片压缩不依赖 `sips`，macOS / Linux / Windows 都能用
- 记录格式和网站前端、fs 助手完全一致：800px 宽、质量 60 的 JPEG，`status: pending`，金额 0

使用前提：电脑能访问公司内网（`10.1.2.247`）。

## 安装

**给同事（macOS，推荐）**：在仓库里运行 `sh fsr/package.sh`，生成 `fsr/dist/fsr-macos.zip`（通用二进制，Apple 芯片和 Intel 都能用），直接发给同事。他们解压后运行 `sh install.sh`，再执行 `fsr login` 即可，不需要装 Go，也不需要仓库权限。

**从源码安装**：

需要 [Go 1.26+](https://go.dev/dl/) 和这个仓库的访问权限（私有仓库，找 mozm 开通）。

macOS / Linux：

```sh
git clone https://github.com/mozm-ctrl/fs-reimburse.git
sh fs-reimburse/fsr/install.sh
```

Windows（PowerShell）：

```powershell
git clone https://github.com/mozm-ctrl/fs-reimburse.git
powershell -ExecutionPolicy Bypass -File fs-reimburse\fsr\install.ps1
```

安装脚本会：

1. 编译 `fsr`，放到 `~/.local/bin`（Windows 放到 `%LOCALAPPDATA%\fsr`，并加入 PATH）
2. 把 `SKILL.md` 复制到 `~/.claude/skills/fs-reimburse/`，Claude Code 会自动加载

其他智能体可以用 `--skills-dir`（PowerShell 用 `-SkillsDir`）指定 skill 目录；也可以直接把 `SKILL.md` 的内容加到该智能体的 skill 或说明里。智能体运行的那台电脑上必须装好 `fsr` 并已登录。

## 登录

在自己的终端里运行一次：

```sh
fsr login
```

按提示输入邮箱和密码（密码不会显示，也不会保存）。token 过期后智能体会提示你重新运行这条命令。退出登录：`fsr logout`。

## 使用

对智能体说：

- “帮我把这张小票按餐饮报销上传” + 图片路径
- “这两张是采购，用途是办公用品，帮我报销”
- “看看我最近的报销记录”

智能体会先预检，并向你确认账号、类型、日期和张数，你同意后才提交，最后告诉你记录 ID。

也可以自己直接用：

```sh
fsr whoami                                             # 当前登录的账号
fsr upload --dry-run receipt.jpg                       # 只检查，不提交
fsr upload receipt.jpg                                 # 餐饮，日期为今天
fsr upload --type purchase --memo "办公用品" a.jpg b.png  # 采购，最多 5 张
fsr records -n 10                                      # 最近的记录
```

`upload` 和 `records` 加 `--json` 会输出 JSON，方便智能体解析。

### 规则

- 类型：`meal`（餐饮，默认）或 `purchase`（采购，可以带用途 `--memo`）
- 日期固定为当天；网站要求当天先有饭否/采购打卡，否则会拒绝，`fsr` 会原样显示网站的报错
- 一条记录 1–5 张图；支持 JPEG / PNG / GIF / WebP / BMP / TIFF。HEIC 请先转成 JPEG。手机照片的方向会按 EXIF 自动转正

### 配置

| 变量 | 说明 |
| --- | --- |
| `INTRANET_URL` | 报销网站地址，默认 `http://10.1.2.247`。`fsr login --url` 也可以设置，会保存到 token 文件里 |
| `FSR_CONFIG` | token 文件路径，默认 `~/.config/fs-reimburse/auth.json`（设置了 `XDG_CONFIG_HOME` 时放在其下） |

## 开发

```sh
cd fsr
go test ./...
GOOS=windows GOARCH=amd64 go build -o fsr.exe .   # 交叉编译
```

| 文件 | 作用 |
| --- | --- |
| `main.go` | 命令行入口，token 文件读写 |
| `intranet.go` | 报销网站（PocketBase）登录、刷新、创建/查询记录，与上层 `intranet.go` 一致 |
| `image.go` | 纯 Go 图片压缩和 EXIF 方向处理 |
| `SKILL.md` | 给智能体看的操作说明 |

`fsr` 是独立的 Go 模块，不影响上层的飞书 fs 助手服务。
