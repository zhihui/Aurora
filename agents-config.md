# 注意
没有配置文件可新增配置文件，有配置文件要在原配置文件值的基础上新增或更新配置

# opencode
## 配置文件
编辑 OpenCode 的配置文件，路径如下：
macOS / Linux：~/.config/opencode/opencode.json
Windows：C:\Users\您的用户名\.config\opencode\opencode.json

## 配置
```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "volcengine-plan/ark-code-latest",
  "provider": {
    "volcengine-plan": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Volcano Engine",
      "options": {
        "baseURL": "https://ark.cn-beijing.volces.com/api/coding/v3",
        "apiKey": "<ARK_API_KEY>"
      },
      "models": {
        "glm-5.2": {
          "name": "glm-5.2",
          "limit": {
            "context": 1024000,
            "output": 4096
          },
          "modalities": {
            "input": [
              "text"
            ],
            "output": [
              "text"
            ]
          }
        },
        "kimi-k2.7-code": {
          "name": "kimi-k2.7-code",
          "limit": {
            "context": 256000,
            "output": 4096
          },
          "modalities": {
            "input": [
              "text"
            ],
            "output": [
              "text"
            ]
          }
        }
      }
    }
  }
}
```

# claude code 
## 配置文件
macOS/Linux 系统 Claude Code 配置文件路径：~/.claude/settings.json。
Windows 系统 Claude Code 配置文件路径：C:\Users\<用户名>\.claude\settings.json

## 配置
```toml
{
    "env": {
        "ANTHROPIC_AUTH_TOKEN": "<ARK_API_KEY>",
        "ANTHROPIC_BASE_URL": "https://ark.cn-beijing.volces.com/api/coding",
        "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "1000000",
        "ANTHROPIC_MODEL": "glm-5.2[1m]",
        "ANTHROPIC_DEFAULT_HAIKU_MODEL": "glm-5.2[1m]",
        "ANTHROPIC_DEFAULT_SONNET_MODEL": "glm-5.2[1m]",
        "ANTHROPIC_DEFAULT_OPUS_MODEL": "glm-5.2[1m]"
    }
}
```

开启 1M 上下文时，需要在模型名称后增加 [1m] 后缀，如 glm-5.2[1m]，同时配置压缩窗口大小参数 "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "1000000"。

## 更新配置后同时更新此陪孩子
编辑或新增 .claude.json 文件，修改或新增 hasCompletedOnboarding 字段值为 true。
不同系统.claude.json配置文件路径如下：
macOS/Linux：~/.claude.json
Windows：C:\Users\<用户名>\.claude.json
修改或新增的字段信息如下：
```json
{
  "hasCompletedOnboarding": true
}
```


# kimi code
## 配置文件
~/.kimi-code/config.toml

## 配置

```toml
default_model = "bytedance/glm-5.2"
# 定义 provider
[providers.bytedance]
type = "openai" # 兼容 claude 的API 值为 anthropic
base_url = "https://ark.cn-beijing.volces.com/api/coding/v3"
api_key = ""

# provider 的模型
[models."bytedance/glm-5.2"]
provider = "bytedance"
model = "glm-5.2"
max_context_size = 131072
capabilities = [ "image_in", "video_in", "thinking" ]
display_name = "GLM-5.2"

# provider 的模型
[models."bytedance/kimi-k2.7-code"]
provider = "bytedance"
model = "kimi-k2.7-code"
max_context_size = 32768
capabilities = [ "image_in", "video_in", "thinking" ]
display_name = "Kimi-k2.7-code"
```

# codex

## 配置文件
~/.codex/config.toml

## 配置
```toml
model = "glm-5.2"
model_provider = "bytedance"
# model_context_window = 

[model_providers.bytedance]
name = "Byte Dance"
base_url = "https://ark.cn-beijing.volces.com/api/coding/v3"
env_key = "BYTEDANCE_MODEL_API_KEY" # BYTEDANCE_MODEL_API_KEY 为环境变量
wire_api = "responses"
```
