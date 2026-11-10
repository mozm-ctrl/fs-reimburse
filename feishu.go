package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Thin wrappers over the Feishu OpenAPI calls the service and CLI need.

var ownerChat = func() string {
	if c := os.Getenv("FEISHU_OWNER_CHAT"); c != "" {
		return c
	}
	return "oc_32078232305f15e14226a8927e0f165d"
}()

func newLarkClient() *lark.Client {
	return lark.NewClient(os.Getenv("FEISHU_APP_ID"), os.Getenv("FEISHU_APP_SECRET"))
}

func sendText(cli *lark.Client, idType, id, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	res, err := cli.Im.V1.Message.Create(context.Background(), larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(idType).
		Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(id).MsgType("text").Content(string(content)).Build()).
		Build())
	if err != nil {
		return err
	}
	if !res.Success() {
		return fmt.Errorf("飞书错误 %d: %s", res.Code, res.Msg)
	}
	return nil
}

func downloadImage(cli *lark.Client, messageID, key string) ([]byte, error) {
	res, err := cli.Im.V1.MessageResource.Get(context.Background(), larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).FileKey(key).Type("image").Build())
	if err != nil {
		return nil, err
	}
	if !res.Success() {
		return nil, fmt.Errorf("下载图片失败 %d: %s", res.Code, res.Msg)
	}
	return io.ReadAll(res.File)
}

// msgContent is the union of the text / image / post message payloads.
type msgContent struct {
	Text     string `json:"text"`
	ImageKey string `json:"image_key"`
	Content  [][]struct {
		Tag      string `json:"tag"`
		Text     string `json:"text"`
		ImageKey string `json:"image_key"`
	} `json:"content"`
}

// parseContent returns the image keys and the text (caption) of a message.
func parseContent(msgType, raw string) (keys []string, text string) {
	var c msgContent
	_ = json.Unmarshal([]byte(raw), &c)
	switch msgType {
	case "text":
		return nil, c.Text
	case "image":
		return []string{c.ImageKey}, ""
	case "post":
		for _, line := range c.Content {
			for _, el := range line {
				if el.Tag == "img" {
					keys = append(keys, el.ImageKey)
				}
				if el.Tag == "text" {
					text += el.Text
				}
			}
		}
	}
	return keys, text
}

type reminder struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

func loadReminders(path string) ([]reminder, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []reminder
	return list, json.Unmarshal(raw, &list)
}

func saveReminders(path string, list []reminder) error {
	if list == nil {
		list = []reminder{}
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

func lookupOpenID(cli *lark.Client, who string) (string, error) {
	body := larkcontact.NewBatchGetIdUserReqBodyBuilder()
	if containsAt(who) {
		body.Emails([]string{who})
	} else {
		body.Mobiles([]string{who})
	}
	res, err := cli.Contact.V3.User.BatchGetId(context.Background(), larkcontact.NewBatchGetIdUserReqBuilder().
		UserIdType("open_id").Body(body.Build()).Build())
	if err != nil {
		return "", err
	}
	if !res.Success() {
		return "", fmt.Errorf("飞书错误 %d: %s", res.Code, res.Msg)
	}
	if len(res.Data.UserList) == 0 || res.Data.UserList[0].UserId == nil {
		return "", fmt.Errorf("找不到 %s：对方可能不在 fs助手 的可用范围内，或不在同一飞书组织", who)
	}
	return *res.Data.UserList[0].UserId, nil
}

func containsAt(s string) bool {
	for _, r := range s {
		if r == '@' {
			return true
		}
	}
	return false
}
