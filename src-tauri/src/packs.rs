use crate::paths;
use serde::{Deserialize, Serialize};

/// One skill pack as stored in packs.json.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredPack {
    pub name: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub skills: Vec<String>,
}

/// Load all packs from packs.json (missing file → empty list).
pub fn load() -> Result<Vec<StoredPack>, String> {
    let file = paths::packs_file()?;
    if !file.exists() {
        return Ok(Vec::new());
    }
    let raw = std::fs::read_to_string(&file).map_err(|e| format!("读取 packs.json 失败: {e}"))?;
    if raw.trim().is_empty() {
        return Ok(Vec::new());
    }
    serde_json::from_str(&raw).map_err(|e| format!("解析 packs.json 失败: {e}"))
}

/// Persist packs to packs.json.
pub fn save(packs: &[StoredPack]) -> Result<(), String> {
    paths::ensure_hub()?;
    let file = paths::packs_file()?;
    let json = serde_json::to_string_pretty(packs).map_err(|e| format!("序列化失败: {e}"))?;
    std::fs::write(&file, json).map_err(|e| format!("写入 packs.json 失败: {e}"))
}
