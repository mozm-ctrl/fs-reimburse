# fs-reimburse

飞书机器人“fs 助手”的报销凭证转发服务。在飞书里给 fs 助手发小票图片，它会自动替你在公司报销网站（nodelink-intranet，`http://10.1.2.247`）创建一条待审核的报销记录，并回复记录 ID。

- 支持多人使用：每个人绑定自己的报销网站账号，提交到各自名下
- 只保存登录 token（AES-256-GCM 加密），不保存密码
- 用飞书长连接接收消息，不需要公网 IP 或内网穿透
- Go 编写，编译后是单个可执行文件

> **最简单（macOS，零安装）**：[`skill/fs-reimburse/`](skill/fs-reimburse/SKILL.md) 是纯说明书版 skill，只用 Mac 自带的 curl/sips/jq，由大模型照着执行，不需要安装任何程序。把这个文件夹放进 `~/.claude/skills/`，或在 Multica 里绑定给智能体即可。
>
> 不想依赖这台 Mac 上的服务？[`fsr/`](fsr/README.md) 是同样功能的独立命令行工具和智能体 skill，每个人在自己电脑上安装，让 AI 智能体帮你上传报销。

## 使用方法（普通用户）

在飞书里搜索 **fs 助手**，发起单聊（群聊里 @ 它无效）。

| 你发送 | 效果 |
| --- | --- |
| `绑定 邮箱 密码` | 首次使用时绑定报销网站账号。成功后请**撤回这条消息**，密码不会被保存 |
| 一张或多张图片 | 按 **餐饮**（`MEAL`）提交，日期为当天，最多 5 张 |
| `采购 用途`，然后发图片 | 下一次图片按 **采购**（`SHOPPING_RECEIPT`）提交，并附上用途 |
| 图文消息，文字写 `采购 用途` | 同上，一条消息完成 |
| `餐饮` | 取消之前的“采购”设置 |
| `解绑` | 删除绑定 |
| 其他文字 | 显示帮助和当前绑定的账号 |

提交后金额为 0，由管理员审核时填写，与网站前端行为一致。

## 部署（管理员）

### 前置条件

- 一台 **macOS** 机器，能访问报销网站 `10.1.2.247`，也能访问飞书（图片压缩用 macOS 自带的 `sips`）
- Go 1.22+（仅编译时需要）
- 飞书开发者后台的企业自建应用：
  - 开启机器人能力
  - 权限：接收/发送单聊消息、获取消息中的资源文件；如需 `feishu send` 命令，还需要通过邮箱/手机号获取用户 ID
  - **事件与回调 → 订阅方式** 选“使用长连接接收事件”，添加事件 `im.message.receive_v1`（需先启动服务，后台才能检测到连接）
  - **版本管理与发布 → 可用范围** 加入要使用的同事，然后发布

### 安装

```sh
git clone https://github.com/mozm-ctrl/fs-reimburse.git
cd fs-reimburse
go build -o fs-reimburse .

INSTALL=~/fs-reimburse
mkdir -p $INSTALL/logs
cp fs-reimburse deploy/start.sh deploy/fsctl.sh $INSTALL/
cp deploy/.env.example $INSTALL/.env && chmod 600 $INSTALL/.env
# 编辑 $INSTALL/.env，填写 FEISHU_APP_ID / FEISHU_APP_SECRET，BIND_KEY 用 openssl rand -hex 32 生成

sed "s#__INSTALL_DIR__#$INSTALL#g" deploy/it.mozm.fs-reimburse.plist > ~/Library/LaunchAgents/it.mozm.fs-reimburse.plist
$INSTALL/fsctl.sh start
$INSTALL/fsctl.sh logs   # 看到 "connected to wss://..." 即成功
```

launchd 会在开机时自动启动服务，进程退出后自动重启。服务只在这台 Mac 开机并能访问内网时可用。

### 管理命令

```sh
fsctl.sh status | start | stop | restart
fsctl.sh logs [n] | errors [n]
fsctl.sh bindings                         # 已绑定的邮箱（不输出 token）
fsctl.sh records <email> [n]              # 该用户最近的报销记录
fsctl.sh upload <email> <MEAL|SHOPPING_RECEIPT> <memo|-> <image>...   # 用本地图片代为提交
fsctl.sh feishu recent [n]                # 管理员与 fs 助手单聊的最近消息
fsctl.sh feishu fetch <message_id> <dir>  # 下载某条消息里的图片
fsctl.sh feishu send-me <text>            # 以 fs 助手身份给管理员发消息
fsctl.sh feishu send <email|mobile> <text>
fsctl.sh feishu remind "YYYY-MM-DD HH:MM" <text>   # 北京时间，到点由服务发给管理员
```

### 配置项（`.env`）

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `FEISHU_APP_ID` / `FEISHU_APP_SECRET` | 是 | 飞书应用凭证 |
| `BIND_KEY` | 是 | 64 位十六进制，加密 `bindings.enc`。修改后所有人需要重新绑定 |
| `INTRANET_URL` | 否 | 报销网站地址，默认 `http://10.1.2.247` |
| `FEISHU_OWNER_CHAT` | 否 | 管理员与 fs 助手单聊的 chat_id，用于提醒和 `send-me` |
| `FSR_DIR` | 否 | 数据目录，默认为可执行文件所在目录 |

运行时数据（`bindings.enc`、`reminders.json`、`logs/`）都在安装目录，已在 `.gitignore` 中排除，**不要提交到仓库**。

## 实现说明

```
飞书单聊 ──长连接事件──▶ fs-reimburse ──下载原图──▶ sips 压缩(800px, JPEG q60)
                              │
                              └─ auth-refresh 续期 token ─▶ POST /api/collections/records/records
```

| 文件 | 作用 |
| --- | --- |
| `main.go` | 命令行入口：`serve` 和各管理命令 |
| `serve.go` | 飞书事件处理：指令解析、去重、图片提交、定时提醒 |
| `intranet.go` | 报销网站（PocketBase）登录、刷新、创建/查询记录，图片编码 |
| `feishu.go` | 飞书消息发送、图片下载、消息内容解析 |
| `store.go` | 加密绑定存储，文件格式为 `IV(12) | tag(16) | 密文`，与早期 Node 版本兼容 |

提交的记录格式与网站前端完全一致：`receiptUrl` 是 JPEG data URL 数组的 JSON 字符串，`status` 为 `pending`，`reimbursementAmount` 为 0。

已知限制：

- “采购”设置和消息去重只保存在内存中，服务重启后会丢失
- 日期取服务器当天，不是小票上的日期
- 网站“当天需先有饭否/采购打卡”的规则由网站自行判断

## 开发

```sh
go test ./...   # 需要 macOS（sips）和 node（绑定文件兼容性测试）
```
