package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Talks to the nodelink-intranet PocketBase backend exactly the way its web frontend does.

var intranet = func() string {
	if u := os.Getenv("INTRANET_URL"); u != "" {
		return u
	}
	return "http://10.1.2.247"
}()

var httpClient = &http.Client{Timeout: 60 * time.Second}

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Auth struct {
	Token  string `json:"token"`
	Record User   `json:"record"`
}

type Record struct {
	ID                  string  `json:"id"`
	Date                string  `json:"date"`
	Type                string  `json:"type"`
	Status              string  `json:"status"`
	ReimbursementAmount float64 `json:"reimbursementAmount"`
	Memo                string  `json:"memo"`
}

func postJSON(path, token string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", intranet+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	return httpClient.Do(req)
}

// Login returns nil auth (no error) when the site rejects the credentials.
func Login(email, password string) (*Auth, error) {
	res, err := postJSON("/api/collections/users/auth-with-password", "", map[string]string{"identity": email, "password": password})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, nil
	}
	var a Auth
	return &a, json.NewDecoder(res.Body).Decode(&a)
}

// Refresh extends the token's lifetime; returns nil auth (no error) when the token has expired.
func Refresh(token string) (*Auth, error) {
	res, err := postJSON("/api/collections/users/auth-refresh", token, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, nil
	}
	var a Auth
	return &a, json.NewDecoder(res.Body).Decode(&a)
}

// ToDataURL uses the site frontend's encoding: 800px wide JPEG, quality 0.6, as a data URL.
func ToDataURL(img []byte) (string, error) {
	dir, err := os.MkdirTemp("", "fsr-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	src, out := filepath.Join(dir, "in"), filepath.Join(dir, "out.jpg")
	if err := os.WriteFile(src, img, 0o600); err != nil {
		return "", err
	}
	if err := exec.Command("sips", "-s", "format", "jpeg", "-s", "formatOptions", "60", "--resampleWidth", "800", src, "--out", out).Run(); err != nil {
		return "", fmt.Errorf("图片转换失败: %w", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b), nil
}

func today() string { return time.Now().Format("2006-01-02") }

func CreateRecord(a *Auth, dataURLs []string, typ, memo string) (*Record, error) {
	receipts, _ := json.Marshal(dataURLs)
	body := map[string]any{
		"userId": a.Record.ID, "userEmail": a.Record.Email, "userName": a.Record.Name,
		"date": today(), "type": typ, "status": "pending", "reimbursementAmount": 0,
		"receiptUrl": string(receipts),
	}
	if typ == "SHOPPING_RECEIPT" && memo != "" {
		body["memo"] = memo
	}
	res, err := postJSON("/api/collections/records/records", a.Token, body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("提交失败 %d: %s", res.StatusCode, msg)
	}
	var rec Record
	return &rec, json.NewDecoder(res.Body).Decode(&rec)
}

func ListRecords(a *Auth, n string) ([]Record, error) {
	q := url.Values{"filter": {fmt.Sprintf(`userId="%s"`, a.Record.ID)}, "sort": {"-created"}, "perPage": {n}}
	req, err := http.NewRequest("GET", intranet+"/api/collections/records/records?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", a.Token)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct{ Items []Record }
	return out.Items, json.NewDecoder(res.Body).Decode(&out)
}
