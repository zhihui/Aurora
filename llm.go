package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// llmConfig is the saved LLM API configuration, in llm.json. The key
// never shows on the page after it is saved.
type llmConfig struct {
	Type     string `json:"type"` // "openai" or "anthropic"
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"`
}

// llmTypes are the API protocols the form offers, with the official
// endpoint each prefills; any compatible third-party endpoint works the
// same.
var llmTypes = []struct {
	id, label, endpoint string
}{
	{"openai", "OpenAI 兼容", "https://api.openai.com/v1"},
	{"anthropic", "Anthropic 兼容", "https://api.anthropic.com"},
}

// llmTypeIndex is the type's place in llmTypes, 0 for an unknown one.
func llmTypeIndex(id string) int {
	for i, lt := range llmTypes {
		if lt.id == id {
			return i
		}
	}
	return 0
}

// llmTypeLabel is the type as the page shows it.
func llmTypeLabel(id string) string {
	return llmTypes[llmTypeIndex(id)].label
}

// loadLLM reads the saved configuration; llmPath is "" in tests, which
// stay in memory.
func (a *app) loadLLM() {
	if a.llmPath == "" {
		return
	}
	data, err := os.ReadFile(a.llmPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.llm)
}

// saveLLM persists the configuration, readable by the user only: it
// holds a key.
func (a *app) saveLLM() {
	if a.llmPath == "" {
		return
	}
	data, err := json.MarshalIndent(a.llm, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.llmPath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(a.llmPath, append(data, '\n'), 0o600)
}

// editLLM opens the form with the saved values; the key field starts
// empty, which keeps the saved key.
func (a *app) editLLM() {
	a.llmEdit = true
	a.llmTypeIdx = llmTypeIndex(a.llm.Type)
	a.llmEndpoint = a.llm.Endpoint
	a.llmModel = a.llm.Model
	a.llmKey = ""
}

// saveLLMConfig takes the form's values: the type, endpoint and model
// always, the key where the field holds a new one.
func (a *app) saveLLMConfig() {
	a.llm.Type = llmTypes[a.llmTypeIdx].id
	a.llm.Endpoint = strings.TrimSpace(a.llmEndpoint)
	a.llm.Model = strings.TrimSpace(a.llmModel)
	if k := strings.TrimSpace(a.llmKey); k != "" {
		a.llm.APIKey = k
	}
	a.saveLLM()
	a.llmEdit = false
	a.llmKey = ""
}

// maskedKey is the saved key as the page shows it: a fixed run of dots,
// telling neither the key nor its length.
const maskedKey = "••••••••"

// llmSection is the settings' 大语言模型 API block: the saved
// configuration as rows — the key masked — or the form editing it.
func (a *app) llmSection(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "大语言模型 API").FontSize(13).Bold()
		})
		ui.Text(c, "用于技能翻译；密钥只保存在本机，保存后不再明文显示。").FontSize(12).TextColor(t.TextMuted)
		if a.llmEdit || a.llm.Type == "" {
			a.llmForm(c)
		} else {
			a.llmSummary(c)
		}
	})
}

// llmSummary is the saved configuration: 类型, 接口 and the masked key,
// with 更改 opening the form.
func (a *app) llmSummary(c *ui.Context) {
	t := c.Theme()
	list := ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Clip()
	list.Children(func() {
		a.llmRow(c, "类型", func() {
			ui.Text(c, llmTypeLabel(a.llm.Type)).FontSize(13).SingleLine()
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "模型", func() {
			ui.Text(c, a.llm.Model).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "接口", func() {
			ui.Text(c, a.llm.Endpoint).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "密钥", func() {
			ui.Text(c, maskedKey).Font("monospace").FontSize(12).TextColor(t.TextMuted).SingleLine()
		}, func() {
			linkAction(c, "更改", false, func() { a.editLLM() })
		})
	})
}

// llmRow is one row of the configuration's container: a muted label and
// the content, with an optional action at the trailing edge.
func (a *app) llmRow(c *ui.Context, label string, content, action func()) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(10, 14).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(40).Shrink(0).SingleLine()
		content()
		ui.Box(c).Grow(1)
		if action != nil {
			action()
		}
	})
}

// llmDivider is the hairline between the container's rows, inset to the
// content's leading edge.
func (a *app) llmDivider(c *ui.Context) {
	ui.Row(c).Children(func() {
		ui.Box(c).Width(66).Shrink(0)
		ui.Box(c).Grow(1).Height(1).Background(c.Theme().Border)
	})
}

// llmForm edits the configuration: the protocol as a segmented control
// — its choice prefills the official endpoint until the user writes one
// — the endpoint, and the key as a password field. 保存 wants an
// endpoint, and a key unless one is saved already.
func (a *app) llmForm(c *ui.Context) {
	t := c.Theme()
	oldIdx := a.llmTypeIdx
	list := ui.Column(c).Radius(10).Background(t.Surface).Border(1, t.Border).Clip()
	list.Children(func() {
		a.llmRow(c, "类型", func() {
			parts := ui.SegmentedBase(c, &a.llmTypeIdx, len(llmTypes))
			track := parts.Track
			track.AlignItems(ui.Center).Padding(2).Gap(2).Radius(8).Background(t.Background).Label("API 类型")
			track.Children(func() {
				for i, lt := range llmTypes {
					s := parts.Segment(i).Padding(4, 12).Radius(6).Center()
					if i == a.llmTypeIdx {
						s.Background(t.Surface).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
					} else {
						s.TextColor(t.TextMuted)
					}
					s.Children(func() { ui.Text(c, lt.label).FontSize(12).SingleLine() })
					s.OnClick(func() {
						// A preset's endpoint prefills until the user
						// writes one of their own.
						if a.llmEndpoint == "" || a.llmEndpoint == llmTypes[oldIdx].endpoint {
							a.llmEndpoint = llmTypes[i].endpoint
						}
					})
				}
			})
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "接口", func() {
			ui.TextInput(c, &a.llmEndpoint).Grow(1).Placeholder("https://api.openai.com/v1").Label("API 接口地址")
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "模型", func() {
			ui.TextInput(c, &a.llmModel).Grow(1).Placeholder("例如 gpt-4o-mini / claude-haiku-4-5").Label("模型名称")
		}, nil)
		a.llmDivider(c)
		a.llmRow(c, "密钥", func() {
			ui.TextInput(c, &a.llmKey).Grow(1).Password().Placeholder("粘贴 API 密钥").Label("API 密钥")
		}, nil)
		a.llmDivider(c)
		ui.Row(c).AlignItems(ui.Center).Gap(10).Padding(10, 14).Children(func() {
			ui.Text(c, "密钥保存为本机文件（仅本人可读），页面上不再明文显示。").FontSize(11).TextColor(t.TextMuted).Grow(1).SingleLine()
			if a.llmEdit && a.llm.Type != "" {
				ui.Button(c, "取消").OnClick(func() { a.llmEdit = false })
			}
			disabled := strings.TrimSpace(a.llmEndpoint) == "" || strings.TrimSpace(a.llmModel) == "" ||
				(strings.TrimSpace(a.llmKey) == "" && a.llm.APIKey == "")
			ui.PrimaryButton(c, "保存").Disabled(disabled).OnClick(func() { a.saveLLMConfig() })
		})
	})
}
