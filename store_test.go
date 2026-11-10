package main

import (
	"os"
	"os/exec"
	"testing"
)

// Go-written bindings.enc must be readable by the Node version, and vice versa.
func TestStoreCompatWithNode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BIND_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("ou_x", Binding{Email: "a@b.c", Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	js := `const c=require("crypto"),fs=require("fs");const K=Buffer.from(process.env.BIND_KEY,"hex");const r=fs.readFileSync(process.argv[1]+"/bindings.enc");const d=c.createDecipheriv("aes-256-gcm",K,r.subarray(0,12));d.setAuthTag(r.subarray(12,28));const b=JSON.parse(Buffer.concat([d.update(r.subarray(28)),d.final()]));b.ou_y={email:"d@e.f",token:"t2"};const iv=c.randomBytes(12);const e=c.createCipheriv("aes-256-gcm",K,iv);const enc=Buffer.concat([e.update(JSON.stringify(b)),e.final()]);fs.writeFileSync(process.argv[1]+"/bindings.enc",Buffer.concat([iv,e.getAuthTag(),enc]));`
	if out, err := exec.Command("node", "-e", js, dir).CombinedOutput(); err != nil {
		t.Fatalf("node: %v %s", err, out)
	}
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := s2.Get("ou_x"); !ok || b.Token != "tok" {
		t.Fatalf("lost go binding: %+v", b)
	}
	if b, ok := s2.FindByEmail("d@e.f"); !ok || b.Token != "t2" {
		t.Fatalf("missing node binding: %+v", b)
	}
	_ = os.Remove(dir)
}

func TestParseContent(t *testing.T) {
	keys, text := parseContent("post", `{"content":[[{"tag":"img","image_key":"k1"}],[{"tag":"text","text":"采购 打印纸"},{"tag":"img","image_key":"k2"}]]}`)
	if len(keys) != 2 || keys[1] != "k2" || text != "采购 打印纸" {
		t.Fatalf("got %v %q", keys, text)
	}
	if m := captionBuyRe.FindStringSubmatch(text); m == nil || m[1] != "打印纸" {
		t.Fatalf("caption: %v", m)
	}
	if m := bindRe.FindStringSubmatch("绑定 邮箱：a@b.com 密码：x1"); m == nil || m[1] != "a@b.com" || m[2] != "x1" {
		t.Fatalf("bind: %v", m)
	}
}

func TestToDataURL(t *testing.T) {
	out := t.TempDir() + "/x.png"
	if err := exec.Command("sips", "-s", "format", "png", "/System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/GenericDocumentIcon.icns", "--out", out).Run(); err != nil {
		t.Skip("no sample image")
	}
	png, _ := os.ReadFile(out)
	u, err := ToDataURL(png)
	if err != nil || len(u) < 100 || u[:23] != "data:image/jpeg;base64," {
		t.Fatalf("%v %.40s", err, u)
	}
}
