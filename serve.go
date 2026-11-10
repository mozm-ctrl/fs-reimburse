package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// fs 助手 → nodelink-intranet: forwards receipt images sent to the bot as reimbursement records.
// Multi-user: each Feishu user binds their own intranet account once ("绑定 邮箱 密码").

const help = `用法：
1. 首次使用先绑定报销网站账号：绑定 邮箱 密码
   （绑定成功后请撤回那条消息，密码不会被保存）
2. 直接发图片 = 按餐饮提交；发"采购 用途"后再发图 = 按采购提交
3. 解绑：解绑`

var (
	// Tolerates "绑定 邮箱xxx@y.com 密码abc", full-width spaces and colons.
	bindRe       = regexp.MustCompile(`^绑定\s*(?:邮箱)?[:：]?\s*([^\s:：]+@[^\s:：]+)\s+(?:密码)?[:：]?\s*(\S+)$`)
	purchaseRe   = regexp.MustCompile(`^采购\s*(.*)$`)
	captionBuyRe = regexp.MustCompile(`采购\s*(.*)$`)
)

type caption struct{ typ, memo string }

type server struct {
	cli     *lark.Client
	store   *Store
	mu      sync.Mutex
	pending map[string]caption // open_id -> type for the next image(s)
	seen    map[string]bool    // Feishu may redeliver; dedupe by message id
}

func (s *server) reply(chatID, text string) {
	if err := sendText(s.cli, "chat_id", chatID, text); err != nil {
		log.Println("reply:", err)
	}
}

func (s *server) handleText(openID, chatID, text string) {
	if m := bindRe.FindStringSubmatch(text); m != nil {
		auth, err := Login(m[1], m[2])
		if err != nil {
			s.reply(chatID, "绑定失败：连不上报销网站，请稍后再试。")
			return
		}
		if auth == nil {
			s.reply(chatID, fmt.Sprintf("绑定失败：网站拒绝了邮箱 %s 和这个密码，请检查后重试。", m[1]))
			return
		}
		if err := s.store.Set(openID, Binding{Email: auth.Record.Email, Token: auth.Token}); err != nil {
			s.reply(chatID, "绑定失败：保存出错 "+err.Error())
			return
		}
		s.reply(chatID, fmt.Sprintf("绑定成功（%s），以后直接发图就行。\n请撤回刚才那条含密码的消息。", auth.Record.Email))
		return
	}
	if strings.HasPrefix(text, "绑定") {
		s.reply(chatID, "格式：绑定 邮箱 密码\n例如：绑定 zhangsan@nodelink.it abc123")
		return
	}
	if text == "解绑" {
		if err := s.store.Delete(openID); err != nil {
			s.reply(chatID, "解绑失败："+err.Error())
			return
		}
		s.reply(chatID, "已解绑。")
		return
	}
	if m := purchaseRe.FindStringSubmatch(text); m != nil {
		memo := strings.TrimSpace(m[1])
		s.mu.Lock()
		s.pending[openID] = caption{"SHOPPING_RECEIPT", memo}
		s.mu.Unlock()
		msg := "下一张图按采购提交"
		if memo != "" {
			msg += "，用途：" + memo
		}
		s.reply(chatID, msg)
		return
	}
	if text == "餐饮" {
		s.mu.Lock()
		delete(s.pending, openID)
		s.mu.Unlock()
		s.reply(chatID, "下一张图按餐饮提交")
		return
	}
	prefix := ""
	if b, ok := s.store.Get(openID); ok {
		prefix = "已绑定：" + b.Email + "\n"
	}
	s.reply(chatID, prefix+help)
}

func (s *server) handle(ev *larkim.P2MessageReceiveV1) {
	msg, sender := ev.Event.Message, ev.Event.Sender
	if msg == nil || sender == nil || sender.SenderId == nil || deref(msg.ChatType) != "p2p" {
		return
	}
	openID, chatID, messageID := deref(sender.SenderId.OpenId), deref(msg.ChatId), deref(msg.MessageId)
	keys, text := parseContent(deref(msg.MessageType), deref(msg.Content))

	if deref(msg.MessageType) == "text" {
		s.handleText(openID, chatID, strings.TrimSpace(text))
		return
	}
	if len(keys) == 0 {
		return
	}
	b, ok := s.store.Get(openID)
	if !ok {
		s.reply(chatID, "你还没有绑定报销网站账号。\n"+help)
		return
	}

	// Image + text in one message: "采购 <用途>" in the caption overrides the default 餐饮.
	s.mu.Lock()
	opts, has := s.pending[openID]
	delete(s.pending, openID)
	s.mu.Unlock()
	if m := captionBuyRe.FindStringSubmatch(strings.TrimSpace(text)); m != nil {
		opts, has = caption{"SHOPPING_RECEIPT", strings.TrimSpace(m[1])}, true
	}
	if !has {
		opts = caption{typ: "MEAL"}
	}

	auth, err := Refresh(b.Token)
	if err != nil {
		log.Println("refresh:", err)
		s.reply(chatID, "上传失败：连不上报销网站 "+err.Error())
		return
	}
	if auth == nil {
		s.reply(chatID, "登录已过期，请重新发送：绑定 邮箱 密码")
		return
	}
	if err := s.store.Set(openID, Binding{Email: auth.Record.Email, Token: auth.Token}); err != nil {
		log.Println("save token:", err)
	}
	if len(keys) > 5 {
		keys = keys[:5]
	}
	var urls []string
	for _, k := range keys {
		img, err := downloadImage(s.cli, messageID, k)
		if err == nil {
			var u string
			if u, err = ToDataURL(img); err == nil {
				urls = append(urls, u)
				continue
			}
		}
		log.Println("image:", err)
		s.reply(chatID, "上传失败："+err.Error())
		return
	}
	rec, err := CreateRecord(auth, urls, opts.typ, opts.memo)
	if err != nil {
		log.Println("create:", err)
		s.reply(chatID, "上传失败："+err.Error())
		return
	}
	label := "餐饮"
	if opts.typ != "MEAL" {
		label = "采购"
	}
	s.reply(chatID, fmt.Sprintf("已上传 %d 张凭证（%s，%s，%s），记录 ID：%s", len(urls), label, rec.Date, auth.Record.Email, rec.ID))
}

// deliverReminders sends reminders queued by "feishu remind" to the owner's chat once due.
func (s *server) deliverReminders(path string) {
	for range time.Tick(30 * time.Second) {
		list, err := loadReminders(path)
		if err != nil {
			log.Println("reminders:", err)
			continue
		}
		var due, rest []reminder
		for _, r := range list {
			if !r.At.After(time.Now()) {
				due = append(due, r)
			} else {
				rest = append(rest, r)
			}
		}
		if len(due) == 0 {
			continue
		}
		if err := saveReminders(path, rest); err != nil {
			log.Println("reminders:", err)
			continue
		}
		for _, r := range due {
			s.reply(ownerChat, "提醒："+r.Text)
		}
	}
}

func serve(dir string) error {
	store, err := OpenStore(dir)
	if err != nil {
		return err
	}
	s := &server{cli: newLarkClient(), store: store, pending: map[string]caption{}, seen: map[string]bool{}}

	handler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(_ context.Context, ev *larkim.P2MessageReceiveV1) error {
			if ev.Event == nil || ev.Event.Message == nil {
				return nil
			}
			id := deref(ev.Event.Message.MessageId)
			s.mu.Lock()
			dup := s.seen[id]
			s.seen[id] = true
			s.mu.Unlock()
			if !dup {
				// Ack immediately, process async.
				go s.handle(ev)
			}
			return nil
		})

	go s.deliverReminders(dir + "/reminders.json")

	ws := larkws.NewClient(getenv("FEISHU_APP_ID"), getenv("FEISHU_APP_SECRET"),
		larkws.WithEventHandler(handler), larkws.WithLogLevel(larkcore.LogLevelInfo))
	log.Printf("fs 助手 reimburse forwarder started (%d bound users, Go)", len(store.Emails()))
	return ws.Start(context.Background())
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
