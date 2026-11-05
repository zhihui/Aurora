package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The configuration persists and loads back, readable by the user only.
func TestLLMPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm.json")
	a := &app{llmPath: path, llm: llmConfig{Type: "anthropic", Endpoint: "https://api.anthropic.com", APIKey: "sk-ant-secret"}}
	a.saveLLM()
	if st, err := os.Stat(path); err != nil {
		t.Fatalf("no llm.json: %v", err)
	} else if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("llm.json perms = %o, want 600", st.Mode().Perm())
	}

	b := &app{llmPath: path}
	b.loadLLM()
	if b.llm != a.llm {
		t.Errorf("loadLLM = %+v, want %+v", b.llm, a.llm)
	}
}

// Without a saved configuration the form shows; a type's choice
// prefills its endpoint until the user writes one.
func TestLLMFormPrefill(t *testing.T) {
	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	for _, s := range []string{"大语言模型 API", "OpenAI 兼容", "Anthropic 兼容"} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
	if a.llmEndpoint != "" {
		t.Fatalf("endpoint starts %q", a.llmEndpoint)
	}
	if err := tt.Click("Anthropic 兼容"); err != nil {
		t.Fatal(err)
	}
	if a.llmEndpoint != "https://api.anthropic.com" || a.llmTypeIdx != 1 {
		t.Errorf("endpoint %q, type %d", a.llmEndpoint, a.llmTypeIdx)
	}
	// The user's own endpoint survives a type switch.
	a.llmEndpoint = "https://my.gateway/v1"
	if err := tt.Click("OpenAI 兼容"); err != nil {
		t.Fatal(err)
	}
	if a.llmEndpoint != "https://my.gateway/v1" {
		t.Errorf("the user's endpoint was overwritten: %q", a.llmEndpoint)
	}
}

// Saving keeps the key off the page: the summary shows the masked run,
// not the key; 更改 reopens the form, and saving it with an empty key
// field keeps the saved key.
func TestLLMSaveMasksKey(t *testing.T) {
	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	a.llmEndpoint = "https://api.openai.com/v1"
	a.llmModel = "gpt-4o-mini"
	a.llmKey = "sk-supersecret"
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.llm.APIKey != "sk-supersecret" || a.llm.Type != "openai" || a.llm.Model != "gpt-4o-mini" {
		t.Fatalf("llm = %+v", a.llm)
	}
	if !tt.HasText(maskedKey) {
		t.Errorf("no masked key, texts %q", tt.Texts())
	}
	if tt.HasText("sk-supersecret") {
		t.Error("the key shows on the page")
	}

	// 更改 reopens the form; an empty key field keeps the saved key.
	if err := tt.Click("更改"); err != nil {
		t.Fatal(err)
	}
	if !a.llmEdit || a.llmKey != "" {
		t.Fatalf("edit %v, key %q", a.llmEdit, a.llmKey)
	}
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.llm.APIKey != "sk-supersecret" {
		t.Errorf("the empty key field dropped the key: %+v", a.llm)
	}
}

// 保存 waits for an endpoint and a model, and for a key unless one is
// saved.
func TestLLMSaveValidation(t *testing.T) {
	a := &app{selected: "settings", focusSide: "sidebar"}
	tt := ui.NewTester(a.view, 1000, 640)
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.llm.Type != "" {
		t.Errorf("an empty form saved: %+v", a.llm)
	}
	a.llmEndpoint = "https://api.openai.com/v1"
	a.llmKey = "sk-x"
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.llm.Type != "" {
		t.Errorf("saved without a model: %+v", a.llm)
	}
	a.llmModel = "gpt-4o-mini"
	a.llmKey = ""
	tt.Frame()
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if a.llm.Type != "" {
		t.Errorf("saved without a key: %+v", a.llm)
	}
}

// The saved file's key is not echoed anywhere on the settings page.
func TestLLMKeyNeverOnPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "llm.json")
	a := &app{selected: "settings", focusSide: "sidebar", llmPath: path,
		llm: llmConfig{Type: "anthropic", Endpoint: "https://api.anthropic.com", APIKey: "sk-ant-xyz"}}
	a.saveLLM()
	tt := ui.NewTester(a.view, 1000, 640)
	if tt.HasText("sk-ant-xyz") {
		t.Error("the key shows in the summary")
	}
	for _, s := range []string{"Anthropic", "https://api.anthropic.com", maskedKey} {
		if !tt.HasText(s) {
			t.Errorf("no text %q, texts %q", s, tt.Texts())
		}
	}
	if err := tt.Click("更改"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("sk-ant-xyz") {
		t.Error("the key shows in the form")
	}
	// 取消 returns to the summary.
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.llmEdit || !tt.HasText(maskedKey) {
		t.Errorf("edit %v, texts %q", a.llmEdit, tt.Texts())
	}
}
