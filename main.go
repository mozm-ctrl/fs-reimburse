package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Single binary for the service and its admin commands (wrapped by fsctl.sh).
// Credentials come from the environment (.env); tokens and secrets are never printed.
const usage = `usage: fs-reimburse <command>
  serve                                         run the Feishu forwarder
  bindings                                      list bound emails
  records <email> [n]                           recent records of a bound account
  upload <email> <MEAL|SHOPPING_RECEIPT> <memo|-> <image>...   submit local images
  feishu recent [n]                             recent messages in mozm's chat with the bot
  feishu fetch <message_id> <outdir>            download a message's images
  feishu send-me <text>                         send text to mozm as the bot
  feishu send <email|mobile> <text>             send text to someone else as the bot
  feishu remind "<YYYY-MM-DD HH:MM>" <text>     the running service sends it at that time`

func main() {
	log.SetOutput(os.Stdout)
	for _, k := range []string{"FEISHU_APP_ID", "FEISHU_APP_SECRET", "BIND_KEY"} {
		if os.Getenv(k) == "" {
			fail("missing env " + k)
		}
	}
	dir := os.Getenv("FSR_DIR")
	if dir == "" {
		exe, _ := os.Executable()
		dir = filepath.Dir(exe)
	}
	args := os.Args[1:]
	if len(args) == 0 {
		fail(usage)
	}
	var err error
	switch args[0] {
	case "serve":
		err = serve(dir)
	case "bindings":
		err = cmdBindings(dir)
	case "records":
		err = cmdRecords(dir, args[1:])
	case "upload":
		err = cmdUpload(dir, args[1:])
	case "feishu":
		err = cmdFeishu(dir, args[1:])
	default:
		fail(usage)
	}
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func getenv(k string) string { return os.Getenv(k) }

func cmdBindings(dir string) error {
	store, err := OpenStore(dir)
	if err != nil {
		return err
	}
	emails := store.Emails()
	fmt.Printf("%d bound users\n", len(emails))
	for _, e := range emails {
		fmt.Println(e)
	}
	return nil
}

// refreshFor refreshes a bound user's token without writing it back: the running service owns
// bindings.enc, and the old token stays valid.
func refreshFor(dir, email string) (*Auth, error) {
	store, err := OpenStore(dir)
	if err != nil {
		return nil, err
	}
	b, ok := store.FindByEmail(email)
	if !ok {
		return nil, fmt.Errorf("%s 还没有在飞书里绑定报销网站账号", email)
	}
	auth, err := Refresh(b.Token)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, fmt.Errorf("登录凭证已过期，请在飞书里重新发送：绑定 邮箱 密码")
	}
	return auth, nil
}

func cmdRecords(dir string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: records <email> [n]")
	}
	n := "10"
	if len(args) > 1 {
		n = args[1]
	}
	auth, err := refreshFor(dir, args[0])
	if err != nil {
		return err
	}
	recs, err := ListRecords(auth, n)
	if err != nil {
		return err
	}
	for _, r := range recs {
		fmt.Println(strings.Join([]string{r.Date, r.Type, r.Status, strconv.FormatFloat(r.ReimbursementAmount, 'f', -1, 64), r.ID, r.Memo}, "\t"))
	}
	return nil
}

func cmdUpload(dir string, args []string) error {
	if len(args) < 4 || (args[1] != "MEAL" && args[1] != "SHOPPING_RECEIPT") {
		return fmt.Errorf("usage: upload <email> <MEAL|SHOPPING_RECEIPT> <memo|-> <image> [image...]")
	}
	email, typ, memo, images := args[0], args[1], args[2], args[3:]
	if len(images) > 5 {
		return fmt.Errorf("at most 5 images per record")
	}
	if memo == "-" {
		memo = ""
	}
	auth, err := refreshFor(dir, email)
	if err != nil {
		return err
	}
	var urls []string
	for _, f := range images {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		u, err := ToDataURL(b)
		if err != nil {
			return err
		}
		urls = append(urls, u)
	}
	rec, err := CreateRecord(auth, urls, typ, memo)
	if err != nil {
		return err
	}
	fmt.Printf("ok %s %s %s %d image(s)\n", rec.ID, rec.Date, typ, len(images))
	return nil
}

func cmdFeishu(dir string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: feishu recent|fetch|send-me|send|remind")
	}
	cli := newLarkClient()
	ctx := context.Background()
	switch args[0] {
	case "recent":
		n := 20
		if len(args) > 1 {
			n, _ = strconv.Atoi(args[1])
		}
		if n <= 0 || n > 50 {
			n = 50
		}
		res, err := cli.Im.V1.Message.List(ctx, larkim.NewListMessageReqBuilder().
			ContainerIdType("chat").ContainerId(ownerChat).SortType("ByCreateTimeDesc").PageSize(n).Build())
		if err != nil {
			return err
		}
		if !res.Success() {
			return fmt.Errorf("飞书错误 %d: %s", res.Code, res.Msg)
		}
		items := res.Data.Items
		for i := len(items) - 1; i >= 0; i-- {
			m := items[i]
			ms, _ := strconv.ParseInt(deref(m.CreateTime), 10, 64)
			who := "fs助手"
			if m.Sender != nil && deref(m.Sender.SenderType) == "user" {
				who = "我"
			}
			content := ""
			if m.Body != nil {
				content = deref(m.Body.Content)
			}
			keys, text := parseContent(deref(m.MsgType), content)
			cols := []string{time.UnixMilli(ms).Format("2006/1/2 15:04:05"), who, deref(m.MessageId)}
			if len(keys) > 0 {
				cols = append(cols, fmt.Sprintf("[%d张图]", len(keys)))
			}
			if text != "" {
				cols = append(cols, text)
			}
			fmt.Println(strings.Join(cols, "\t"))
		}
	case "fetch":
		if len(args) < 3 {
			return fmt.Errorf("usage: fetch <message_id> <outdir>")
		}
		id, outdir := args[1], args[2]
		res, err := cli.Im.V1.Message.Get(ctx, larkim.NewGetMessageReqBuilder().MessageId(id).Build())
		if err != nil {
			return err
		}
		if !res.Success() {
			return fmt.Errorf("飞书错误 %d: %s", res.Code, res.Msg)
		}
		m := res.Data.Items[0]
		if deref(m.ChatId) != ownerChat {
			return fmt.Errorf("只能读取 mozm 和 fs助手 的单聊消息")
		}
		keys, text := parseContent(deref(m.MsgType), deref(m.Body.Content))
		if len(keys) == 0 {
			return fmt.Errorf("这条消息里没有图片")
		}
		if err := os.MkdirAll(outdir, 0o755); err != nil {
			return err
		}
		for i, k := range keys {
			img, err := downloadImage(cli, id, k)
			if err != nil {
				return err
			}
			file, _ := filepath.Abs(filepath.Join(outdir, fmt.Sprintf("%s-%d.img", id, i+1)))
			if err := os.WriteFile(file, img, 0o644); err != nil {
				return err
			}
			fmt.Println(file)
		}
		if text != "" {
			fmt.Fprintln(os.Stderr, "caption: "+text)
		}
	case "send-me":
		if err := sendText(cli, "chat_id", ownerChat, strings.Join(args[1:], " ")); err != nil {
			return err
		}
		fmt.Println("sent")
	case "send":
		if len(args) < 3 {
			return fmt.Errorf("usage: send <email|mobile> <text>")
		}
		id, err := lookupOpenID(cli, args[1])
		if err != nil {
			return err
		}
		if err := sendText(cli, "open_id", id, strings.Join(args[2:], " ")); err != nil {
			return err
		}
		fmt.Println("sent")
	case "remind":
		if len(args) < 3 {
			return fmt.Errorf(`usage: remind "<YYYY-MM-DD HH:MM>" <text>`)
		}
		at, err := time.ParseInLocation("2006-01-02 15:04", args[1], time.FixedZone("CST", 8*3600))
		if err != nil {
			return fmt.Errorf("时间格式：YYYY-MM-DD HH:MM（北京时间）")
		}
		if at.Before(time.Now()) {
			return fmt.Errorf("提醒时间已经过去了")
		}
		path := filepath.Join(dir, "reminders.json")
		list, err := loadReminders(path)
		if err != nil {
			return err
		}
		list = append(list, reminder{At: at.UTC(), Text: strings.Join(args[2:], " ")})
		if err := saveReminders(path, list); err != nil {
			return err
		}
		fmt.Println("queued for " + at.Format("2006/1/2 15:04:05"))
	default:
		return fmt.Errorf("usage: feishu recent|fetch|send-me|send|remind")
	}
	return nil
}
