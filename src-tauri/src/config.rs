use crate::paths;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use std::path::PathBuf;

/// LLM provider config as stored in ~/.aurora/config.json.
/// The api_key is persisted here but never returned to the frontend.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct LlmConfig {
    #[serde(default)]
    pub endpoint: String,
    #[serde(default)]
    pub model: String,
    #[serde(default)]
    pub api_key: String,
}

/// A model registered under a provider. The `id` is what gets sent to the API.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Model {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    /// Optional context window size in tokens.
    #[serde(default)]
    pub context_window: Option<u64>,
    /// Optional maximum output length in tokens.
    #[serde(default)]
    pub max_output: Option<u64>,
    /// Modalities the model supports. "text" is always present (locked on);
    /// "image" and "video" are optional. Old configs without this field
    /// deserialize to just ["text"].
    #[serde(default = "default_model_capabilities")]
    pub capabilities: Vec<String>,
}

impl Default for Model {
    fn default() -> Self {
        Self {
            id: String::new(),
            name: String::new(),
            context_window: None,
            max_output: None,
            capabilities: default_model_capabilities(),
        }
    }
}

/// A model provider. `key` is the stable identifier (lowercase, unique); the
/// `api_key` is persisted but never returned to the frontend.
///
/// A provider may expose two distinct API styles: an OpenAI-compatible
/// (Chat Completions) endpoint and a Claude-compatible (Messages) endpoint.
/// Either may be left empty when the provider doesn't support that style.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Provider {
    #[serde(default)]
    pub key: String,
    #[serde(default)]
    pub name: String,
    /// OpenAI-compatible (Chat Completions) base URL, e.g. https://api.openai.com/v1
    #[serde(default, alias = "endpoint")]
    pub endpoint_openai: String,
    /// Claude-compatible (Messages) base URL, e.g. https://api.anthropic.com
    #[serde(default)]
    pub endpoint_claude: String,
    #[serde(default)]
    pub site: String,
    #[serde(default)]
    pub api_key: String,
    #[serde(default)]
    pub models: Vec<Model>,
}

impl Provider {
    /// Stable color derived from the provider key, so each provider gets a
    /// consistent badge color without the user choosing one.
    pub fn color(&self) -> String {
        const PALETTE: [&str; 8] = [
            "#5B5BD6", "#10A37F", "#D97757", "#4D6BFE", "#EAB308", "#6366F1", "#0EA5E9", "#EC4899",
        ];
        let mut hash: u64 = 0xcbf29ce484222325;
        for b in self.key.as_bytes() {
            hash ^= *b as u64;
            hash = hash.wrapping_mul(0x100000001b3);
        }
        PALETTE[(hash as usize) % PALETTE.len()].to_string()
    }

    /// First two non-space, non-punct characters of the name, uppercased — used
    /// for the list/detail avatar when no icon exists.
    pub fn initials(&self) -> String {
        let cleaned: String = self.name.chars().filter(|c| c.is_alphanumeric()).collect();
        let chars: Vec<char> = cleaned.chars().take(2).collect();
        if chars.is_empty() {
            return "··".to_string();
        }
        chars.into_iter().collect::<String>().to_uppercase()
    }
}

/// Claude Code-specific options for generated settings.json env keys.
///
/// The `[1m]` model-name suffix and `CLAUDE_CODE_AUTO_COMPACT_WINDOW` are
/// decided automatically from the selected model's `context_window` (>= 1M),
/// so there is no toggle for them here. `~/.claude.json` is always written with
/// `hasCompletedOnboarding: true`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClaudeAgentModelOptions {
    #[serde(default = "default_true")]
    pub same_model_for_slots: bool,
    #[serde(default)]
    pub haiku_model: String,
    #[serde(default)]
    pub sonnet_model: String,
    #[serde(default)]
    pub opus_model: String,
}

impl Default for ClaudeAgentModelOptions {
    fn default() -> Self {
        Self {
            same_model_for_slots: true,
            haiku_model: String::new(),
            sonnet_model: String::new(),
            opus_model: String::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KimiAgentModelOptions {
    #[serde(default = "default_openai_type")]
    pub provider_type: String,
    #[serde(default = "default_kimi_capabilities")]
    pub capabilities: Vec<String>,
}

impl Default for KimiAgentModelOptions {
    fn default() -> Self {
        Self {
            provider_type: default_openai_type(),
            capabilities: default_kimi_capabilities(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OpencodeAgentModelOptions {
    /// Compatibility type chosen by the user: "openai" or "anthropic".
    /// Decides the npm package and which provider endpoint is used as baseURL.
    #[serde(default = "default_openai_type")]
    pub provider_type: String,
}

impl Default for OpencodeAgentModelOptions {
    fn default() -> Self {
        Self {
            provider_type: default_openai_type(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CodexAgentModelOptions {
    #[serde(default)]
    pub env_key: String,
}

impl Default for CodexAgentModelOptions {
    fn default() -> Self {
        Self {
            env_key: String::new(),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AgentModelOptions {
    #[serde(default)]
    pub claude: ClaudeAgentModelOptions,
    #[serde(default)]
    pub kimi: KimiAgentModelOptions,
    #[serde(default)]
    pub opencode: OpencodeAgentModelOptions,
    #[serde(default)]
    pub codex: CodexAgentModelOptions,
}

/// Stores only a reference into the model center. Generated native agent configs
/// are resolved from the current provider/model at write time.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AgentModelConfig {
    #[serde(default)]
    pub provider: String,
    #[serde(default)]
    pub model: String,
    #[serde(default)]
    pub options: AgentModelOptions,
}

fn default_true() -> bool {
    true
}
fn default_openai_type() -> String {
    "openai".to_string()
}
fn default_kimi_capabilities() -> Vec<String> {
    vec![
        "image_in".to_string(),
        "video_in".to_string(),
        "thinking".to_string(),
    ]
}

/// Canonical model modalities. "text" is always present; "image" and "video"
/// are optional. Stored in this fixed order.
pub fn default_model_capabilities() -> Vec<String> {
    vec!["text".to_string()]
}

/// Normalize a capabilities list: keep only the known modalities, always
/// include "text", dedupe, and order as text → image → video.
pub fn normalize_model_capabilities(caps: &[String]) -> Vec<String> {
    let order = ["text", "image", "video"];
    let mut present = [false; 3];
    for c in caps {
        let t = c.trim();
        if let Some(idx) = order.iter().position(|o| *o == t) {
            present[idx] = true;
        }
    }
    present[0] = true; // text is always on
    order
        .iter()
        .zip(present.iter())
        .filter(|(_, &on)| on)
        .map(|(o, _)| o.to_string())
        .collect()
}

/// A user-added agent skill directory. Built-in agents (claude/codex/opencode/
/// agents/kimi) are not stored here — only extras the user added from the
/// candidate list. `rel_dir` is relative to the user's home directory.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct CustomAgent {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub color: String,
    #[serde(default)]
    pub rel_dir: String,
}

/// Top-level config.json document. Kept as an object so future settings can be
/// added without breaking existing files.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Config {
    #[serde(default)]
    pub llm: LlmConfig,
    #[serde(default)]
    pub providers: Vec<Provider>,
    #[serde(default)]
    pub agent_models: BTreeMap<String, AgentModelConfig>,
    /// User-added agents (beyond the built-in set).
    #[serde(default)]
    pub custom_agents: Vec<CustomAgent>,
    /// Built-in agent ids the user has explicitly removed (e.g. "kimi"), so
    /// they don't reappear on restart. The 4 core built-ins can't be removed.
    #[serde(default)]
    pub removed_builtin: Vec<String>,
}

pub fn load() -> Result<Config, String> {
    let file = paths::config_file()?;
    if !file.exists() {
        return Ok(Config::default());
    }
    let raw = std::fs::read_to_string(&file).map_err(|e| format!("读取 config.json 失败: {e}"))?;
    if raw.trim().is_empty() {
        return Ok(Config::default());
    }
    serde_json::from_str(&raw).map_err(|e| format!("解析 config.json 失败: {e}"))
}

pub fn save(config: &Config) -> Result<(), String> {
    paths::ensure_hub()?;
    let file = paths::config_file()?;
    let json = serde_json::to_string_pretty(config).map_err(|e| format!("序列化失败: {e}"))?;
    std::fs::write(&file, json).map_err(|e| format!("写入 config.json 失败: {e}"))
}

/// Validate a provider key: non-empty, lowercase alnum / dash / underscore only.
pub fn validate_provider_key(key: &str) -> Result<(), String> {
    if key.is_empty() {
        return Err("英文标识不能为空".to_string());
    }
    if !key
        .chars()
        .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-' || c == '_')
    {
        return Err("英文标识仅限小写字母、数字、- _".to_string());
    }
    Ok(())
}

// ─────────────────────────── translation cache ───────────────────────────

/// A cached Chinese translation, tied to a hash of the source SKILL.md so it is
/// transparently invalidated when the source changes.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CachedTranslation {
    pub source_hash: String,
    pub text: String,
}

/// ~/.aurora/cache/translations/<name>.json
fn translation_cache_file(name: &str) -> Result<PathBuf, String> {
    Ok(paths::hub_root()?
        .join("cache")
        .join("translations")
        .join(format!("{name}.json")))
}

/// A cheap, stable content hash (FNV-1a) — no external crate needed.
pub fn content_hash(s: &str) -> String {
    let mut hash: u64 = 0xcbf29ce484222325;
    for b in s.as_bytes() {
        hash ^= *b as u64;
        hash = hash.wrapping_mul(0x100000001b3);
    }
    format!("{hash:016x}")
}

/// Return the cached translation iff it matches the current source hash.
pub fn read_cached_translation(name: &str, source_hash: &str) -> Option<String> {
    let file = translation_cache_file(name).ok()?;
    let raw = std::fs::read_to_string(&file).ok()?;
    let cached: CachedTranslation = serde_json::from_str(&raw).ok()?;
    (cached.source_hash == source_hash).then_some(cached.text)
}

pub fn write_cached_translation(name: &str, source_hash: &str, text: &str) -> Result<(), String> {
    let file = translation_cache_file(name)?;
    if let Some(parent) = file.parent() {
        std::fs::create_dir_all(parent).map_err(|e| format!("创建缓存目录失败: {e}"))?;
    }
    let payload = CachedTranslation {
        source_hash: source_hash.to_string(),
        text: text.to_string(),
    };
    let json = serde_json::to_string_pretty(&payload).map_err(|e| format!("序列化失败: {e}"))?;
    std::fs::write(&file, json).map_err(|e| format!("写入缓存失败: {e}"))
}
