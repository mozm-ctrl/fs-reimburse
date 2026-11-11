---
name: fs-reimburse
description: Submit reimbursement receipts (meal or purchase) to the company reimbursement site nodelink-intranet (10.1.2.247) with the local `fsr` CLI. Use when the user asks to upload, submit or file a receipt / 小票 / 发票 / 报销凭证, says things like "帮我把这张小票按餐饮报销上传" or "采购报销", or asks about their recent reimbursement records.
---

# 报销上传（fs-reimburse）

用本机的 `fsr` 命令把小票图片提交到公司报销网站，生成一条“待审核”记录。金额由管理员审核时填写，不需要你填。

## 准备检查

1. 运行 `fsr version`。如果找不到命令，告诉用户按 README 安装（`fsr/README.md`），然后停下。
2. 运行 `fsr whoami`，确认已登录，并记下邮箱。
   - 报“还没有登录”或“登录已过期”：请用户**在自己的终端里**运行 `fsr login`（Claude Code 里可以输入 `! fsr login`）。**不要在对话里向用户要密码**，也不要替用户输入密码。用户登录完成后，再运行一次 `fsr whoami`。
   - 报“连不上报销网站”：说明这台电脑现在访问不到公司内网，请用户连上公司网络或 VPN 后再试。

## 确定参数

- **类型**：默认是餐饮（`--type meal`）。只有用户明确说是采购、买东西、办公用品等，才用采购（`--type purchase`），并带上用途 `--memo "<用途>"`。用户说采购但没说用途时，问一句用途；用户不想写也可以不带。餐饮不带用途。
- **日期**：自动取今天，不能改。如果用户说的是以前的小票，提醒他记录日期会是今天。
- **图片**：用户给的本地图片路径。一条记录 1–5 张；超过 5 张时，先问用户是拆成多条，还是只挑 5 张。
  - 支持 JPEG / PNG / GIF / WebP / BMP / TIFF。iPhone 的 HEIC 需要先转换：macOS 上用 `sips -s format jpeg in.heic --out out.jpg`，其他系统请用户导出为 JPEG。
  - 一张图里有几张小票、或者多张图属于同一笔报销，都放在同一条记录里。

## 提交前确认（必须）

提交会在报销网站上生成真实记录，所以要先向用户确认：

1. 先预检：`fsr upload --dry-run --type <meal|purchase> [--memo "<用途>"] <图片...>`。这一步会检查登录和图片，但不会提交。
2. 把结果告诉用户，例如：“将以 zhangsan@nodelink.it 提交 1 张凭证：餐饮，日期 2026-11-11。确认提交吗？”
3. 用户明确同意后才正式提交。用户在同一句话里已经说了“直接提交，不用确认”的，可以跳过这一步。

## 提交

```sh
fsr upload --json --type meal <图片...>
fsr upload --json --type purchase --memo "办公用品" <图片...>
```

成功时会输出 JSON，包含 `id`（记录 ID）、`date`、`type`、`status`。把记录 ID、类型和日期告诉用户。同一组图片只提交一次：如果不确定上次有没有成功，先用 `fsr records` 查一下，不要直接重复提交。

## 查询记录

`fsr records -n 10`（或加 `--json`），按时间倒序列出自己的记录，格式是：日期、类型、状态、金额、记录 ID、用途。

## 出错时

| 错误信息 | 怎么处理 |
| --- | --- |
| 还没有登录 / 登录已过期 | 请用户在自己的终端运行 `fsr login`，完成后重试 |
| 登录失败：网站拒绝了邮箱…和这个密码 | 请用户检查邮箱和密码，重新运行 `fsr login` |
| 连不上报销网站 | 请用户连上公司内网或 VPN 后重试 |
| 报销网站返回 400 …（比如当天还没有饭否/采购打卡） | 把网站原话转告用户。按网站规则，当天要先打卡，请用户先去网站打卡，再让你重试 |
| 无法识别的图片格式 | 按上面的方法转成 JPEG 后重试 |
| 一条记录需要 1-5 张图片 | 按“图片”一节拆分 |

不要修改或删除 `~/.config/fs-reimburse/auth.json`，也不要打印其中的 token。用户想退出登录时，运行 `fsr logout`。
