package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// translationFile is the file holding a skill's Chinese translation,
// inside the skill's directory; it lives and dies with the skill, as
// the import record does.
const translationFile = ".aurora-zh.json"

// translation is what translationFile holds: the Chinese rendering of
// the skill's SKILL.md, and the hash of what was translated — a SKILL.md
// that changed since has a different hash, and the translation is
// stale.
type translation struct {
	SourceHash   string    `json:"sourceHash"`
	Model        string    `json:"model"`
	TranslatedAt time.Time `json:"translatedAt"`
	Content      string    `json:"content"`
}

// skillHash is the fingerprint a translation is tied to.
func skillHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// loadTranslation reads the skill's translation, false where it has
// none.
func loadTranslation(dir string) (*translation, bool) {
	data, err := os.ReadFile(filepath.Join(dir, translationFile))
	if err != nil {
		return nil, false
	}
	var tr translation
	if json.Unmarshal(data, &tr) != nil || tr.Content == "" {
		return nil, false
	}
	return &tr, true
}

// saveTranslation writes the skill's translation.
func saveTranslation(dir string, tr translation) error {
	data, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, translationFile), append(data, '\n'), 0o644)
}

// translateSystem is the instruction the translation goes by: a whole
// SKILL.md to Chinese, its structure untouched.
const translateSystem = "你是专业的技术文档翻译。把用户给出的 SKILL.md 完整翻译成简体中文。保持 Markdown 结构不变；front matter 的键名、name 值、代码块、命令、链接与文件路径不译；description 值翻译。只输出译文，不要任何解释或评论。"

// translateMaxLen is the most of a SKILL.md sent to the model.
const translateMaxLen = 200 * 1024

// llmChat asks the configured model for one completion; a variable, so
// the tests can stub it.
var llmChat = chatLLM

// chatLLM calls the configured API: the OpenAI protocol or the
// Anthropic one, as the configuration's type says.
func chatLLM(ctx context.Context, cfg llmConfig, system, user string) (string, error) {
	switch cfg.Type {
	case "anthropic":
		return chatAnthropic(ctx, cfg, system, user)
	default:
		return chatOpenAI(ctx, cfg, system, user)
	}
}

// apiURL joins the configured endpoint and the protocol's path: an
// endpoint already carrying /v1 is kept, one without gets it.
func apiURL(endpoint, version, path string) string {
	e := strings.TrimSuffix(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(e, "/"+version) {
		return e + path
	}
	return e + "/" + version + path
}

// postJSON sends body and decodes the reply; a non-2xx status is an
// error with the body's gist.
func postJSON(ctx context.Context, url string, header http.Header, body, reply any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range header {
		req.Header[k] = vs
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		gist := strings.TrimSpace(string(raw))
		if len(gist) > 200 {
			gist = gist[:200] + "…"
		}
		return fmt.Errorf("API 返回 %s: %s", resp.Status, gist)
	}
	if err := json.Unmarshal(raw, reply); err != nil {
		return fmt.Errorf("无法解析 API 响应: %v", err)
	}
	return nil
}

// chatOpenAI is the OpenAI protocol: POST /chat/completions with a
// bearer key.
func chatOpenAI(ctx context.Context, cfg llmConfig, system, user string) (string, error) {
	body := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	var reply struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	header := http.Header{"Authorization": {"Bearer " + cfg.APIKey}}
	if err := postJSON(ctx, apiURL(cfg.Endpoint, "v1", "/chat/completions"), header, body, &reply); err != nil {
		return "", err
	}
	if len(reply.Choices) == 0 {
		return "", fmt.Errorf("API 响应中没有译文")
	}
	return strings.TrimSpace(reply.Choices[0].Message.Content), nil
}

// chatAnthropic is the Anthropic protocol: POST /messages with an
// x-api-key header.
func chatAnthropic(ctx context.Context, cfg llmConfig, system, user string) (string, error) {
	body := map[string]any{
		"model":      cfg.Model,
		"max_tokens": 8192,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	}
	var reply struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	header := http.Header{
		"x-api-key":         {cfg.APIKey},
		"anthropic-version": {"2023-06-01"},
	}
	if err := postJSON(ctx, apiURL(cfg.Endpoint, "v1", "/messages"), header, body, &reply); err != nil {
		return "", err
	}
	for _, b := range reply.Content {
		if b.Text != "" {
			return strings.TrimSpace(b.Text), nil
		}
	}
	return "", fmt.Errorf("API 响应中没有译文")
}
