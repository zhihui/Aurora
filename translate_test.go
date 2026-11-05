package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The translation file round-trips, and the hash tells a stale one.
func TestTranslationFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := loadTranslation(dir); ok {
		t.Fatal("a translation out of nowhere")
	}
	tr := translation{SourceHash: skillHash([]byte("v1")), Model: "m", TranslatedAt: time.Now(), Content: "译文"}
	if err := saveTranslation(dir, tr); err != nil {
		t.Fatal(err)
	}
	got, ok := loadTranslation(dir)
	if !ok || got.Content != "译文" || got.SourceHash != tr.SourceHash {
		t.Fatalf("loadTranslation = %+v, %v", got, ok)
	}
	if got.SourceHash == skillHash([]byte("v2")) {
		t.Error("a changed SKILL.md keeps the hash")
	}
}

func TestAPIURL(t *testing.T) {
	for endpoint, want := range map[string]string{
		"https://api.openai.com":     "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1":  "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1/": "https://api.openai.com/v1/chat/completions",
		"https://my.gateway/v1":      "https://my.gateway/v1/chat/completions",
		"https://my.gateway/proxy":   "https://my.gateway/proxy/v1/chat/completions",
	} {
		if got := apiURL(endpoint, "v1", "/chat/completions"); got != want {
			t.Errorf("apiURL(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

// The OpenAI protocol: the bearer key, the model and the messages, and
// the answer out of the choices.
func TestChatOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-4o-mini" {
			t.Errorf("model = %v", body["model"])
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"译文内容"}}]}`))
	}))
	defer srv.Close()
	cfg := llmConfig{Type: "openai", Endpoint: srv.URL, Model: "gpt-4o-mini", APIKey: "k"}
	out, err := chatLLM(context.Background(), cfg, "sys", "user")
	if err != nil || out != "译文内容" {
		t.Errorf("chatLLM = %q, %v", out, err)
	}
}

// The Anthropic protocol: x-api-key, the version header, max_tokens,
// and the text out of the content blocks.
func TestChatAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("headers = %v", r.Header)
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "claude-haiku-4-5" || body["max_tokens"] == nil || body["system"] == nil {
			t.Errorf("body = %v", body)
		}
		w.Write([]byte(`{"content":[{"type":"text","text":"译文内容"}]}`))
	}))
	defer srv.Close()
	cfg := llmConfig{Type: "anthropic", Endpoint: srv.URL, Model: "claude-haiku-4-5", APIKey: "k"}
	out, err := chatLLM(context.Background(), cfg, "sys", "user")
	if err != nil || out != "译文内容" {
		t.Errorf("chatLLM = %q, %v", out, err)
	}
}

// A non-2xx reply is an error with the body's gist, not a panic.
func TestChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer srv.Close()
	cfg := llmConfig{Type: "openai", Endpoint: srv.URL, Model: "m", APIKey: "bad"}
	_, err := chatLLM(context.Background(), cfg, "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid key") {
		t.Errorf("err = %v", err)
	}
}
