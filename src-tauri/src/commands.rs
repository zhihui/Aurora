use crate::config;
use crate::import::{self, ImportResult, ImportSelection, ParsedImport};
use crate::meta::read_description;
use crate::packs::{self, StoredPack};
use crate::paths;
use serde::Serialize;
use serde_json::{Map, Value};
use std::path::{Path, PathBuf};
use toml_edit::{value as toml_value, Array as TomlArray, DocumentMut};

/// Create a directory link in a cross-platform way.
/// unix → symlink; windows → junction (no admin / Developer Mode required).
fn make_dir_symlink(target: &Path, link: &Path) -> std::io::Result<()> {
    #[cfg(unix)]
    {
        std::os::unix::fs::symlink(target, link)
    }
    #[cfg(windows)]
    {
        junction::create(target, link)
    }
}

/// Remove a directory link (symlink or junction) without touching its target.
fn remove_link(link: &Path) -> std::io::Result<()> {
    #[cfg(unix)]
    {
        std::fs::remove_file(link)
    }
    #[cfg(windows)]
    {
        // Works for both junctions and directory symlinks on Windows.
        std::fs::remove_dir(link)
    }
}

// ─────────────────────────── DTOs ───────────────────────────

#[derive(Serialize)]
pub struct AgentInfo {
    pub id: String,
    pub name: String,
    pub color: String,
}

#[derive(Serialize)]
pub struct Skill {
    pub name: String,
    pub description: String,
    /// Agent ids this skill is currently linked into (center softlink).
    pub assigned_agents: Vec<String>,
}

#[derive(Serialize)]
pub struct Pack {
    pub name: String,
    pub description: String,
    pub skills: Vec<String>,
    /// Agent ids where every skill in the pack is linked.
    pub assigned_agents: Vec<String>,
}

#[derive(Serialize)]
pub struct AgentSkill {
    pub name: String,
    pub description: String,
    /// "center" | "real" | "external"
    pub source: String,
    /// Where the entry points (for links) or its own path (for real dirs).
    pub target: String,
}

#[derive(Serialize)]
pub struct AgentDirInfo {
    pub id: String,
    pub name: String,
    pub color: String,
    pub path: String,
    pub exists: bool,
    pub skill_count: usize,
    /// Whether the user is allowed to remove this agent entry.
    pub removable: bool,
}

/// LLM config returned to the frontend — the api key is intentionally omitted,
/// only its presence is reported.
#[derive(Serialize)]
pub struct LlmConfigDto {
    pub endpoint: String,
    pub model: String,
    pub has_key: bool,
}

/// A model as seen by the frontend.
#[derive(Serialize)]
pub struct ModelDto {
    pub id: String,
    pub name: String,
    pub context_window: Option<u64>,
    pub max_output: Option<u64>,
    pub capabilities: Vec<String>,
}

/// A provider as seen by the frontend — the api key is omitted, only its
/// presence is reported (`has_key`).
#[derive(Serialize)]
pub struct ProviderDto {
    pub key: String,
    pub name: String,
    pub endpoint_openai: String,
    pub endpoint_claude: String,
    pub site: String,
    pub color: String,
    pub initials: String,
    pub has_key: bool,
    pub models: Vec<ModelDto>,
}

/// Agent model option payload. It mirrors config::AgentModelOptions and is accepted
/// from the frontend; defaults fill agent-specific optional fields.
pub type AgentModelOptionsDto = config::AgentModelOptions;

#[derive(Serialize)]
pub struct AgentModelFileDto {
    pub path: String,
    pub content: String,
}

#[derive(Serialize)]
pub struct AgentModelConfigDto {
    pub agent: AgentInfo,
    pub config_path: String,
    pub secondary_config_path: String,
    pub endpoint_kind: String,
    pub configured: bool,
    pub provider: String,
    pub model: String,
    pub options: AgentModelOptionsDto,
    pub resolved_provider: Option<ProviderDto>,
    pub resolved_model: Option<ModelDto>,
    pub error: String,
}

#[derive(Serialize)]
pub struct AgentModelSyncResult {
    pub agent: String,
    pub ok: bool,
    pub message: String,
}

#[derive(Serialize)]
pub struct TranslationDto {
    pub text: String,
    /// true when served from cache, false when freshly translated.
    pub cached: bool,
}

// ─────────────────────────── helpers ───────────────────────────

/// Agent ids whose skills dir holds a center symlink for `skill`.
fn assigned_agents_for(skill: &str) -> Result<Vec<String>, String> {
    let center = paths::skill_path(skill)?;
    let mut ids = Vec::new();
    for agent in paths::load_agents() {
        let link = agent.skills_dir()?.join(skill);
        if paths::symlink_points_to(&link, &center) {
            ids.push(agent.id);
        }
    }
    Ok(ids)
}

fn copy_dir_all(src: &Path, dst: &Path) -> Result<(), String> {
    std::fs::create_dir_all(dst).map_err(|e| format!("创建目录失败: {e}"))?;
    for entry in std::fs::read_dir(src).map_err(|e| format!("读取目录失败: {e}"))? {
        let entry = entry.map_err(|e| e.to_string())?;
        let ty = entry.file_type().map_err(|e| e.to_string())?;
        let from = entry.path();
        let to = dst.join(entry.file_name());
        if ty.is_dir() {
            copy_dir_all(&from, &to)?;
        } else {
            std::fs::copy(&from, &to).map_err(|e| format!("复制文件失败: {e}"))?;
        }
    }
    Ok(())
}

fn validate_name(name: &str) -> Result<(), String> {
    if name.is_empty() || name.contains('/') || name.contains("..") {
        return Err("名称不合法".to_string());
    }
    Ok(())
}

/// Entries starting with `.` (e.g. .DS_Store, .git) are ignored everywhere.
fn is_hidden(name: &std::ffi::OsStr) -> bool {
    name.to_string_lossy().starts_with('.')
}

// ─────────────────────────── agents ───────────────────────────

#[tauri::command]
pub fn list_agents() -> Vec<AgentInfo> {
    paths::load_agents()
        .into_iter()
        .map(|a| AgentInfo {
            id: a.id,
            name: a.name,
            color: a.color,
        })
        .collect()
}

#[tauri::command]
pub fn list_agent_dirs() -> Result<Vec<AgentDirInfo>, String> {
    let mut out = Vec::new();
    for agent in paths::load_agents() {
        let dir = agent.skills_dir()?;
        let exists = dir.is_dir();
        let skill_count = if exists {
            std::fs::read_dir(&dir)
                .map(|rd| {
                    rd.filter_map(|e| e.ok())
                        .filter(|e| !is_hidden(&e.file_name()))
                        .filter(|e| {
                            let p = e.path();
                            p.is_dir()
                                || std::fs::symlink_metadata(&p)
                                    .map(|m| m.file_type().is_symlink())
                                    .unwrap_or(false)
                        })
                        .count()
                })
                .unwrap_or(0)
        } else {
            0
        };
        out.push(AgentDirInfo {
            removable: paths::is_removable(&agent.id),
            id: agent.id,
            name: agent.name,
            color: agent.color,
            path: dir.to_string_lossy().to_string(),
            exists,
            skill_count,
        });
    }
    Ok(out)
}

#[tauri::command]
pub fn create_agent_dir(agent: String) -> Result<(), String> {
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    std::fs::create_dir_all(a.skills_dir()?).map_err(|e| format!("创建目录失败: {e}"))
}

/// A candidate agent the user may add from the Settings page.
#[derive(Serialize)]
pub struct CandidateAgentDto {
    pub id: String,
    pub name: String,
    pub color: String,
    pub rel_dir: String,
}

/// Candidate agents (from the preset list) not yet in the active list.
#[tauri::command]
pub fn list_candidate_agents() -> Vec<CandidateAgentDto> {
    let loaded = paths::load_agents();
    let active: std::collections::HashSet<&str> = loaded.iter().map(|a| a.id.as_str()).collect();
    paths::CANDIDATE_AGENTS
        .iter()
        .filter(|c| !active.contains(c.id))
        .map(|c| CandidateAgentDto {
            id: c.id.to_string(),
            name: c.name.to_string(),
            color: c.color.to_string(),
            rel_dir: c.rel_dir.to_string(),
        })
        .collect()
}

/// Add a candidate agent by id. The skills directory is created on disk.
#[tauri::command]
pub fn add_agent(id: String) -> Result<AgentDirInfo, String> {
    let id = id.trim().to_string();
    let def = paths::candidate_by_id(&id).ok_or("未知的候选 agent")?;
    let mut cfg = config::load()?;
    // Reject duplicates (already built-in or already added).
    if paths::find_agent(&id).is_some() {
        return Err("该 agent 已存在".to_string());
    }
    // If it was previously removed (e.g. kimi), un-remove it.
    cfg.removed_builtin.retain(|r| r != &id);
    cfg.custom_agents.push(config::CustomAgent {
        id: def.id.to_string(),
        name: def.name.to_string(),
        color: def.color.to_string(),
        rel_dir: def.rel_dir.to_string(),
    });
    config::save(&cfg)?;
    let agent = paths::Agent {
        id: def.id.to_string(),
        name: def.name.to_string(),
        color: def.color.to_string(),
        rel_dir: def.rel_dir.to_string(),
    };
    let dir = agent.skills_dir()?;
    std::fs::create_dir_all(&dir).map_err(|e| format!("创建目录失败: {e}"))?;
    Ok(AgentDirInfo {
        removable: true,
        id: agent.id,
        name: agent.name,
        color: agent.color,
        path: dir.to_string_lossy().to_string(),
        exists: true,
        skill_count: 0,
    })
}

/// Remove an agent entry. Only removable agents (kimi + custom) can be removed;
/// the core 4 built-ins are protected. Center softlinks under the agent's skills
/// directory are cleaned up; the directory itself and external/real entries are
/// left untouched.
#[tauri::command]
pub fn remove_agent(id: String) -> Result<(), String> {
    let id = id.trim().to_string();
    if !paths::is_removable(&id) {
        return Err("该 agent 不可移除".to_string());
    }
    let agent = paths::find_agent(&id).ok_or("未知 agent")?;

    // Clean only center softlinks in this agent's skills dir.
    if let Ok(dir) = agent.skills_dir() {
        if let Ok(rd) = std::fs::read_dir(&dir) {
            for entry in rd.flatten() {
                let p = entry.path();
                if let Some(link_name) = entry.file_name().to_str() {
                    if let Ok(center) = paths::skill_path(link_name) {
                        if paths::symlink_points_to(&p, &center) {
                            let _ = remove_link(&p);
                        }
                    }
                }
            }
        }
    }

    let mut cfg = config::load()?;
    let is_builtin =
        paths::candidate_by_id(&id).is_none() && cfg.custom_agents.iter().all(|c| c.id != id);
    if is_builtin {
        // A built-in removable agent (kimi): record removal so it stays gone.
        if !cfg.removed_builtin.contains(&id) {
            cfg.removed_builtin.push(id);
        }
    } else {
        // User-added: drop from custom_agents.
        cfg.custom_agents.retain(|c| c.id != id);
    }
    config::save(&cfg)
}

// ─────────────────────────── skill center ───────────────────────────

#[tauri::command]
pub fn list_skills() -> Result<Vec<Skill>, String> {
    paths::ensure_hub()?;
    let dir = paths::skills_dir()?;
    let mut skills = Vec::new();
    for entry in std::fs::read_dir(&dir).map_err(|e| format!("读取技能目录失败: {e}"))? {
        let entry = entry.map_err(|e| e.to_string())?;
        if is_hidden(&entry.file_name()) {
            continue;
        }
        let path = entry.path();
        if !path.is_dir() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().to_string();
        skills.push(Skill {
            description: read_description(&path),
            assigned_agents: assigned_agents_for(&name)?,
            name,
        });
    }
    skills.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
    Ok(skills)
}

#[tauri::command]
pub fn read_skill_md(name: String) -> Result<String, String> {
    validate_name(&name)?;
    let md = paths::skill_path(&name)?.join("SKILL.md");
    std::fs::read_to_string(&md).map_err(|_| "该技能没有 SKILL.md".to_string())
}

#[tauri::command]
pub fn create_skill(name: String, description: String) -> Result<(), String> {
    validate_name(&name)?;
    paths::ensure_hub()?;
    let dir = paths::skill_path(&name)?;
    if dir.exists() {
        return Err("技能已存在".to_string());
    }
    std::fs::create_dir_all(&dir).map_err(|e| format!("创建技能失败: {e}"))?;
    let desc = if description.trim().is_empty() {
        "描述这个技能的用途。".to_string()
    } else {
        description.trim().to_string()
    };
    let md = format!("---\nname: {name}\ndescription: {desc}\n---\n\n# {name}\n\n{desc}\n");
    std::fs::write(dir.join("SKILL.md"), md).map_err(|e| format!("写入 SKILL.md 失败: {e}"))
}

/// Delete a center skill: drop it from every pack, remove only the center
/// softlinks in agents (never real dirs or external links), then delete the dir.
#[tauri::command]
pub fn delete_skill(name: String) -> Result<(), String> {
    validate_name(&name)?;
    let center = paths::skill_path(&name)?;

    // 1. Remove from packs.
    let mut all = packs::load()?;
    let mut changed = false;
    for p in &mut all {
        let before = p.skills.len();
        p.skills.retain(|s| s != &name);
        changed |= p.skills.len() != before;
    }
    if changed {
        packs::save(&all)?;
    }

    // 2. Remove center softlinks in each agent (protect real dirs / external links).
    for agent in paths::load_agents() {
        let link = agent.skills_dir()?.join(&name);
        if paths::symlink_points_to(&link, &center) {
            remove_link(&link).map_err(|e| format!("移除软链接失败: {e}"))?;
        }
    }

    // 3. Remove the real directory from the center.
    if center.exists() {
        std::fs::remove_dir_all(&center).map_err(|e| format!("删除技能失败: {e}"))?;
    }
    Ok(())
}

// ─────────────────────────── assign / unassign ───────────────────────────

fn link_skill_into(agent_dir: &Path, name: &str, center: &Path) -> Result<(), String> {
    std::fs::create_dir_all(agent_dir).map_err(|e| format!("创建 agent 目录失败: {e}"))?;
    let link = agent_dir.join(name);
    if paths::symlink_points_to(&link, center) {
        return Ok(()); // already linked
    }
    if link.exists() || std::fs::symlink_metadata(&link).is_ok() {
        return Err(format!("{name}：该 agent 下已存在同名技能，未覆盖"));
    }
    make_dir_symlink(center, &link).map_err(|e| format!("创建软链接失败: {e}"))
}

#[tauri::command]
pub fn assign_skill(skill: String, agent: String) -> Result<(), String> {
    validate_name(&skill)?;
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let center = paths::skill_path(&skill)?;
    if !center.is_dir() {
        return Err("技能中心不存在该技能".to_string());
    }
    link_skill_into(&a.skills_dir()?, &skill, &center)
}

#[tauri::command]
pub fn unassign_skill(skill: String, agent: String) -> Result<(), String> {
    validate_name(&skill)?;
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let center = paths::skill_path(&skill)?;
    let link = a.skills_dir()?.join(&skill);
    // Only remove our own center softlink.
    if paths::symlink_points_to(&link, &center) {
        remove_link(&link).map_err(|e| format!("移除软链接失败: {e}"))?;
    }
    Ok(())
}

// ─────────────────────────── packs ───────────────────────────

fn pack_assigned_agents(skills: &[String]) -> Result<Vec<String>, String> {
    if skills.is_empty() {
        return Ok(Vec::new());
    }
    let mut ids = Vec::new();
    'agents: for agent in paths::load_agents() {
        let base = agent.skills_dir()?;
        for s in skills {
            let center = paths::skill_path(s)?;
            if !paths::symlink_points_to(&base.join(s), &center) {
                continue 'agents;
            }
        }
        ids.push(agent.id);
    }
    Ok(ids)
}

#[tauri::command]
pub fn list_packs() -> Result<Vec<Pack>, String> {
    let stored = packs::load()?;
    let mut out = Vec::new();
    for p in stored {
        out.push(Pack {
            assigned_agents: pack_assigned_agents(&p.skills)?,
            name: p.name,
            description: p.description,
            skills: p.skills,
        });
    }
    Ok(out)
}

#[tauri::command]
pub fn create_pack(name: String, description: String) -> Result<(), String> {
    if name.trim().is_empty() {
        return Err("技能包名称不能为空".to_string());
    }
    let mut all = packs::load()?;
    if all.iter().any(|p| p.name == name) {
        return Err("同名技能包已存在".to_string());
    }
    all.push(StoredPack {
        name,
        description,
        skills: Vec::new(),
    });
    packs::save(&all)
}

#[tauri::command]
pub fn delete_pack(name: String) -> Result<(), String> {
    let mut all = packs::load()?;
    all.retain(|p| p.name != name);
    packs::save(&all)
}

#[tauri::command]
pub fn rename_pack(name: String, new_name: String, description: String) -> Result<(), String> {
    if new_name.trim().is_empty() {
        return Err("技能包名称不能为空".to_string());
    }
    let mut all = packs::load()?;
    if new_name != name && all.iter().any(|p| p.name == new_name) {
        return Err("同名技能包已存在".to_string());
    }
    let p = all
        .iter_mut()
        .find(|p| p.name == name)
        .ok_or("技能包不存在")?;
    p.name = new_name;
    p.description = description;
    packs::save(&all)
}

#[tauri::command]
pub fn add_skill_to_pack(pack: String, skill: String) -> Result<(), String> {
    let mut all = packs::load()?;
    let p = all
        .iter_mut()
        .find(|p| p.name == pack)
        .ok_or("技能包不存在")?;
    if p.skills.contains(&skill) {
        return Ok(()); // already in pack
    }
    // Agents that currently have this pack fully assigned (based on the
    // pre-add skill set) — they must receive the new skill too, otherwise
    // the pack would silently appear "unassigned" after the edit.
    let assigned = pack_assigned_agents(&p.skills)?;
    p.skills.push(skill.clone());
    packs::save(&all)?;

    let center = paths::skill_path(&skill)?;
    if center.is_dir() {
        let mut errors = Vec::new();
        for id in &assigned {
            if let Some(a) = paths::find_agent(id) {
                if let Ok(dir) = a.skills_dir() {
                    if let Err(e) = link_skill_into(&dir, &skill, &center) {
                        errors.push(e);
                    }
                }
            }
        }
        if !errors.is_empty() {
            return Err(format!(
                "已加入技能包，但同步到已分配 agent 失败：{}",
                errors.join("；")
            ));
        }
    }
    Ok(())
}

#[tauri::command]
pub fn remove_skill_from_pack(pack: String, skill: String) -> Result<(), String> {
    let mut all = packs::load()?;
    let idx = all
        .iter()
        .position(|p| p.name == pack)
        .ok_or("技能包不存在")?;
    if !all[idx].skills.contains(&skill) {
        return Ok(()); // not in pack
    }
    // Agents that carried this skill via this pack before removal.
    let assigned = pack_assigned_agents(&all[idx].skills)?;
    all[idx].skills.retain(|s| s != &skill);
    packs::save(&all)?;

    // Clean up the now-orphaned link — but keep it if another pack still
    // assigned to the same agent needs this skill.
    let center = paths::skill_path(&skill)?;
    for id in &assigned {
        let still_needed = all.iter().any(|p| {
            p.name != pack
                && p.skills.contains(&skill)
                && pack_assigned_agents(&p.skills)
                    .map(|a| a.contains(id))
                    .unwrap_or(false)
        });
        if still_needed {
            continue;
        }
        if let Some(a) = paths::find_agent(id) {
            if let Ok(dir) = a.skills_dir() {
                let link = dir.join(&skill);
                if paths::symlink_points_to(&link, &center) {
                    remove_link(&link).map_err(|e| format!("移除软链接失败: {e}"))?;
                }
            }
        }
    }
    Ok(())
}

#[tauri::command]
pub fn assign_pack(pack: String, agent: String) -> Result<(), String> {
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let all = packs::load()?;
    let p = all.iter().find(|p| p.name == pack).ok_or("技能包不存在")?;
    let dir = a.skills_dir()?;
    let mut errors = Vec::new();
    for s in &p.skills {
        let center = paths::skill_path(s)?;
        if !center.is_dir() {
            errors.push(format!("{s}：技能中心已无此技能"));
            continue;
        }
        if let Err(e) = link_skill_into(&dir, s, &center) {
            errors.push(e);
        }
    }
    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("；"))
    }
}

#[tauri::command]
pub fn unassign_pack(pack: String, agent: String) -> Result<(), String> {
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let all = packs::load()?;
    let p = all.iter().find(|p| p.name == pack).ok_or("技能包不存在")?;
    let dir = a.skills_dir()?;
    for s in &p.skills {
        let center = paths::skill_path(s)?;
        let link = dir.join(s);
        if paths::symlink_points_to(&link, &center) {
            remove_link(&link).map_err(|e| format!("移除软链接失败: {e}"))?;
        }
    }
    Ok(())
}

// ─────────────────────────── agent skills view ───────────────────────────

#[tauri::command]
pub fn list_agent_skills(agent: String) -> Result<Vec<AgentSkill>, String> {
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let dir = a.skills_dir()?;
    let mut out = Vec::new();
    if !dir.is_dir() {
        return Ok(out);
    }
    let skills_root = paths::skills_dir()?;
    for entry in std::fs::read_dir(&dir).map_err(|e| format!("读取目录失败: {e}"))? {
        let entry = entry.map_err(|e| e.to_string())?;
        if is_hidden(&entry.file_name()) {
            continue;
        }
        let path = entry.path();
        let name = entry.file_name().to_string_lossy().to_string();
        if std::fs::symlink_metadata(&path).is_err() {
            continue;
        }

        // A symlink or Windows junction is a link; a plain directory is "real".
        let (source, target, desc_dir): (&str, String, PathBuf) =
            if let Some(raw) = paths::read_any_link(&path) {
                let resolved = if raw.is_absolute() {
                    raw.clone()
                } else {
                    path.parent().map(|p| p.join(&raw)).unwrap_or(raw.clone())
                };
                let canon = resolved.canonicalize().unwrap_or(resolved.clone());
                let is_center =
                    canon.starts_with(skills_root.canonicalize().unwrap_or(skills_root.clone()));
                (
                    if is_center { "center" } else { "external" },
                    canon.to_string_lossy().to_string(),
                    canon,
                )
            } else if path.is_dir() {
                ("real", path.to_string_lossy().to_string(), path.clone())
            } else {
                continue;
            };

        out.push(AgentSkill {
            name,
            description: read_description(&desc_dir),
            source: source.to_string(),
            target,
        });
    }
    out.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
    Ok(out)
}

/// Remove a skill from an agent. Only center softlinks are removed; real dirs
/// and external links are protected and left untouched (returns an error).
#[tauri::command]
pub fn remove_agent_skill(agent: String, name: String) -> Result<(), String> {
    validate_name(&name)?;
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let center = paths::skill_path(&name)?;
    let link = a.skills_dir()?.join(&name);
    if paths::symlink_points_to(&link, &center) {
        remove_link(&link).map_err(|e| format!("移除软链接失败: {e}"))
    } else {
        Err("仅可移除技能中心的软链接；真实目录与外部链接受保护".to_string())
    }
}

/// Import an agent's real-dir or external-link skill into the center, then
/// replace the original entry with a center softlink so it becomes managed.
#[tauri::command]
pub fn import_skill(agent: String, name: String) -> Result<(), String> {
    validate_name(&name)?;
    paths::ensure_hub()?;
    let a = paths::find_agent(&agent).ok_or("未知 agent")?;
    let entry = a.skills_dir()?.join(&name);
    if std::fs::symlink_metadata(&entry).is_err() {
        return Err("该技能不存在".to_string());
    }
    let link_target = paths::read_any_link(&entry);
    let is_link = link_target.is_some();

    // Resolve the directory whose contents we copy into the center.
    let source: PathBuf = if let Some(raw) = link_target {
        let resolved = if raw.is_absolute() {
            raw
        } else {
            entry.parent().map(|p| p.join(&raw)).unwrap_or(raw)
        };
        resolved
            .canonicalize()
            .map_err(|e| format!("解析链接失败: {e}"))?
    } else if entry.is_dir() {
        entry.clone()
    } else {
        return Err("不支持导入该项".to_string());
    };

    let dest = paths::skill_path(&name)?;
    if dest.exists() {
        return Err("技能中心已存在同名技能".to_string());
    }
    copy_dir_all(&source, &dest)?;

    // Replace the original entry with a center link.
    if is_link {
        remove_link(&entry).map_err(|e| format!("移除原链接失败: {e}"))?;
    } else {
        std::fs::remove_dir_all(&entry).map_err(|e| format!("移除原目录失败: {e}"))?;
    }
    make_dir_symlink(&dest, &entry).map_err(|e| format!("创建软链接失败: {e}"))
}

// ─────────────────────────── llm config ───────────────────────────

#[tauri::command]
pub fn get_llm_config() -> Result<LlmConfigDto, String> {
    let cfg = config::load()?;
    Ok(LlmConfigDto {
        endpoint: cfg.llm.endpoint,
        model: cfg.llm.model,
        has_key: !cfg.llm.api_key.is_empty(),
    })
}

/// Save endpoint/model. When `api_key` is `None` the stored key is kept as-is;
/// pass `Some("")` to explicitly clear it.
#[tauri::command]
pub fn set_llm_config(
    endpoint: String,
    model: String,
    api_key: Option<String>,
) -> Result<(), String> {
    let mut cfg = config::load()?;
    cfg.llm.endpoint = endpoint.trim().to_string();
    cfg.llm.model = model.trim().to_string();
    if let Some(key) = api_key {
        cfg.llm.api_key = key.trim().to_string();
    }
    config::save(&cfg)
}

// ─────────────────────────── agent model config ───────────────────────────

const MODEL_AGENTS: &[(&str, &str, &str, &str, &str)] = &[
    (
        "claude",
        "Claude Code",
        "#D97757",
        "~/.claude/settings.json",
        "claude",
    ),
    (
        "kimi",
        "Kimi Code",
        "#6366F1",
        "~/.kimi-code/config.toml",
        "openai",
    ),
    (
        "opencode",
        "Opencode",
        "#EAB308",
        "~/.config/opencode/opencode.jsonc",
        "openai",
    ),
    (
        "codex",
        "Codex",
        "#10A37F",
        "~/.codex/config.toml",
        "openai",
    ),
];

fn model_agent_meta(
    agent: &str,
) -> Option<(
    &'static str,
    &'static str,
    &'static str,
    &'static str,
    &'static str,
)> {
    MODEL_AGENTS
        .iter()
        .copied()
        .find(|(id, _, _, _, _)| *id == agent)
}

fn expand_home(path: &str) -> Result<PathBuf, String> {
    if let Some(rest) = path.strip_prefix("~/") {
        Ok(paths::home_dir()?.join(rest))
    } else {
        Ok(PathBuf::from(path))
    }
}

fn agent_config_path(agent: &str) -> Result<PathBuf, String> {
    let (_, _, _, rel, _) = model_agent_meta(agent).ok_or("未知 Agent")?;
    expand_home(rel)
}

fn claude_onboarding_path() -> Result<PathBuf, String> {
    Ok(paths::home_dir()?.join(".claude.json"))
}

fn agent_info_for_model(id: &str) -> AgentInfo {
    let (id, name, color, _, _) = model_agent_meta(id).unwrap();
    AgentInfo {
        id: id.to_string(),
        name: name.to_string(),
        color: color.to_string(),
    }
}

fn model_to_dto(m: &config::Model) -> ModelDto {
    ModelDto {
        id: m.id.clone(),
        name: m.name.clone(),
        context_window: m.context_window,
        max_output: m.max_output,
        capabilities: config::normalize_model_capabilities(&m.capabilities),
    }
}

/// Resolve and validate a provider+model reference. `options` is used for
/// agents whose compatibility type is user-selectable (Kimi may be `openai` or
/// `anthropic`): only the endpoint matching the chosen type is required then.
fn validate_agent_ref<'a>(
    cfg: &'a config::Config,
    agent: &str,
    provider: &str,
    model: &str,
    options: &config::AgentModelOptions,
) -> Result<(&'a config::Provider, &'a config::Model), String> {
    let (_, _, _, _, endpoint_kind) = model_agent_meta(agent).ok_or("未知 Agent")?;
    let p = cfg
        .providers
        .iter()
        .find(|p| p.key == provider)
        .ok_or("Provider 不存在")?;
    let m = p
        .models
        .iter()
        .find(|m| m.id == model)
        .ok_or("模型不存在")?;
    // For Kimi and Opencode the compatibility type is chosen by the user, so
    // only the endpoint for the selected type must be present.
    let (need_openai, need_claude) = if agent == "kimi" {
        match options.kimi.provider_type.trim() {
            "anthropic" => (false, true),
            _ => (true, false),
        }
    } else if agent == "opencode" {
        match options.opencode.provider_type.trim() {
            "anthropic" => (false, true),
            _ => (true, false),
        }
    } else {
        (endpoint_kind == "openai", endpoint_kind == "claude")
    };
    if need_claude && p.endpoint_claude.trim().is_empty() {
        return Err("该 Provider 未配置 Claude 兼容端点".to_string());
    }
    if need_openai && p.endpoint_openai.trim().is_empty() {
        return Err("该 Provider 未配置 OpenAI 兼容端点".to_string());
    }
    if agent != "codex" && p.api_key.trim().is_empty() {
        return Err("该 Provider 尚未保存 API 密钥".to_string());
    }
    Ok((p, m))
}

fn read_json_object(path: &Path) -> Result<Map<String, Value>, String> {
    if !path.exists() {
        return Ok(Map::new());
    }
    let raw = std::fs::read_to_string(path).map_err(|e| format!("读取配置失败: {e}"))?;
    if raw.trim().is_empty() {
        return Ok(Map::new());
    }
    match serde_json::from_str::<Value>(&raw).map_err(|e| format!("解析 JSON 失败: {e}"))? {
        Value::Object(map) => Ok(map),
        _ => Err("配置文件必须是 JSON 对象".to_string()),
    }
}

fn write_json_object(path: &Path, obj: Map<String, Value>) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| format!("创建配置目录失败: {e}"))?;
    }
    let raw = serde_json::to_string_pretty(&Value::Object(obj))
        .map_err(|e| format!("序列化 JSON 失败: {e}"))?;
    std::fs::write(path, raw).map_err(|e| format!("写入配置失败: {e}"))
}

fn read_toml_doc(path: &Path) -> Result<DocumentMut, String> {
    if !path.exists() {
        return Ok(DocumentMut::new());
    }
    let raw = std::fs::read_to_string(path).map_err(|e| format!("读取配置失败: {e}"))?;
    if raw.trim().is_empty() {
        return Ok(DocumentMut::new());
    }
    raw.parse::<DocumentMut>()
        .map_err(|e| format!("解析 TOML 失败: {e}"))
}

fn write_toml_doc(path: &Path, doc: DocumentMut) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| format!("创建配置目录失败: {e}"))?;
    }
    std::fs::write(path, doc.to_string()).map_err(|e| format!("写入配置失败: {e}"))
}

/// Whether the model's recorded context window qualifies for Claude Code's
/// 1M-context beta: the model center stores a context window and it is at least
/// one million tokens. When true, the `[1m]` name suffix and
/// `CLAUDE_CODE_AUTO_COMPACT_WINDOW` are applied automatically.
fn has_1m_context(model: &config::Model) -> bool {
    model
        .context_window
        .map(|c| c >= 1_000_000)
        .unwrap_or(false)
}

/// Append the `[1m]` suffix when 1M context is enabled and not already present.
fn model_with_1m(model: &str, enable: bool) -> String {
    if enable && !model.ends_with("[1m]") {
        format!("{model}[1m]")
    } else {
        model.to_string()
    }
}

/// Build the `env` entries Claude Code's settings.json should carry for the
/// selected provider/model. Shared by `apply_claude` (real key, merged into the
/// existing file) and `preview_for` (key masked for display) so the two cannot
/// drift. `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is included only when the model
/// qualifies for 1M context.
fn build_claude_env(
    provider: &config::Provider,
    model: &config::Model,
    options: &config::ClaudeAgentModelOptions,
) -> Map<String, Value> {
    let enable_1m = has_1m_context(model);
    let selected = model_with_1m(&model.id, enable_1m);
    // The `[1m]` suffix is decided per slot, from that slot model's own context
    // window — a Haiku slot should not gain `[1m]` merely because the main
    // Sonnet model supports 1M. Slot models are resolved within the same
    // provider's model list (the slot selector only offers those).
    let slot = |custom: &str| -> String {
        let raw = custom.trim();
        if options.same_model_for_slots || raw.is_empty() {
            selected.clone()
        } else {
            let slot_1m = provider
                .models
                .iter()
                .find(|m| m.id == raw)
                .map(has_1m_context)
                .unwrap_or(false);
            model_with_1m(raw, slot_1m)
        }
    };
    let mut env = Map::new();
    env.insert(
        "ANTHROPIC_AUTH_TOKEN".to_string(),
        Value::String(provider.api_key.clone()),
    );
    env.insert(
        "ANTHROPIC_BASE_URL".to_string(),
        Value::String(provider.endpoint_claude.clone()),
    );
    env.insert(
        "ANTHROPIC_MODEL".to_string(),
        Value::String(selected.clone()),
    );
    env.insert(
        "ANTHROPIC_DEFAULT_HAIKU_MODEL".to_string(),
        Value::String(slot(&options.haiku_model)),
    );
    env.insert(
        "ANTHROPIC_DEFAULT_SONNET_MODEL".to_string(),
        Value::String(slot(&options.sonnet_model)),
    );
    env.insert(
        "ANTHROPIC_DEFAULT_OPUS_MODEL".to_string(),
        Value::String(slot(&options.opus_model)),
    );
    if enable_1m {
        env.insert(
            "CLAUDE_CODE_AUTO_COMPACT_WINDOW".to_string(),
            Value::String("1000000".to_string()),
        );
    }
    env
}

fn env_key_for_provider(provider: &str) -> String {
    let key: String = provider
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() {
                c.to_ascii_uppercase()
            } else {
                '_'
            }
        })
        .collect();
    format!("{key}_MODEL_API_KEY")
}

fn apply_claude(
    provider: &config::Provider,
    model: &config::Model,
    options: &config::ClaudeAgentModelOptions,
) -> Result<(), String> {
    let path = agent_config_path("claude")?;
    let mut root = read_json_object(&path)?;
    let env = root
        .entry("env".to_string())
        .or_insert_with(|| Value::Object(Map::new()));
    let Some(env) = env.as_object_mut() else {
        return Err("~/.claude/settings.json 的 env 必须是对象".to_string());
    };
    for (k, v) in build_claude_env(provider, model, options) {
        env.insert(k, v);
    }
    // When the model no longer qualifies for 1M context, drop a previously
    // written compaction window so stale state does not linger.
    if !has_1m_context(model) {
        env.remove("CLAUDE_CODE_AUTO_COMPACT_WINDOW");
    }
    write_json_object(&path, root)?;

    // Always skip Claude Code's first-run onboarding.
    let onb_path = claude_onboarding_path()?;
    let mut onb = read_json_object(&onb_path)?;
    onb.insert("hasCompletedOnboarding".to_string(), Value::Bool(true));
    write_json_object(&onb_path, onb)?;
    Ok(())
}

fn apply_kimi(
    provider: &config::Provider,
    model: &config::Model,
    options: &config::KimiAgentModelOptions,
) -> Result<(), String> {
    let path = agent_config_path("kimi")?;
    let mut doc = read_toml_doc(&path)?;
    let ptype = options.provider_type.trim();
    // base_url follows the selected compatibility type: anthropic uses the
    // Claude-compatible endpoint, everything else the OpenAI-compatible one.
    let base_url = if ptype == "anthropic" {
        provider.endpoint_claude.clone()
    } else {
        provider.endpoint_openai.clone()
    };

    // The selected model becomes the default. Catalog every model from this
    // provider under [models."provider/<id>"] so all of them are selectable
    // inside Kimi Code — not just the default one.
    let default_key = format!("{}/{}", provider.key, model.id);
    doc["default_model"] = toml_value(default_key.clone());

    doc["providers"][&provider.key]["type"] = toml_value(ptype);
    doc["providers"][&provider.key]["base_url"] = toml_value(base_url);
    doc["providers"][&provider.key]["api_key"] = toml_value(provider.api_key.clone());

    // Write every current model, then remove stale model tables so the
    // [models] section always matches the current provider exactly.
    let valid_keys: std::collections::HashSet<String> = provider
        .models
        .iter()
        .map(|m| format!("{}/{}", provider.key, m.id))
        .collect();
    for m in &provider.models {
        let key = format!("{}/{}", provider.key, m.id);
        doc["models"][&key]["provider"] = toml_value(provider.key.clone());
        doc["models"][&key]["model"] = toml_value(m.id.clone());
        if let Some(ctx) = m.context_window {
            doc["models"][&key]["max_context_size"] = toml_value(ctx as i64);
        }
        doc["models"][&key]["display_name"] = toml_value(if m.name.is_empty() {
            m.id.clone()
        } else {
            m.name.clone()
        });
        let mut arr = TomlArray::new();
        for cap in &options.capabilities {
            if !cap.trim().is_empty() {
                arr.push(cap.trim());
            }
        }
        doc["models"][&key]["capabilities"] = toml_value(arr);
    }
    if let Some(models_table) = doc.get_mut("models").and_then(|v| v.as_table_mut()) {
        let stale: Vec<String> = models_table
            .iter()
            .map(|(k, _)| k.to_string())
            .filter(|k| !valid_keys.contains(k))
            .collect();
        for k in stale {
            models_table.remove(&k);
        }
    }
    write_toml_doc(&path, doc)
}

/// Build the OpenCode `modalities` object for a model from its capabilities.
/// `text` is always present (input + output); `image`/`video` add to the input
/// array when the model declares them. Output stays text-only (these models
/// generate text). Shared by apply_opencode and preview_for so they cannot drift.
fn opencode_modalities(model: &config::Model) -> Map<String, Value> {
    let caps = config::normalize_model_capabilities(&model.capabilities);
    let mut input = vec![Value::String("text".to_string())];
    if caps.iter().any(|c| c == "image") {
        input.push(Value::String("image".to_string()));
    }
    if caps.iter().any(|c| c == "video") {
        input.push(Value::String("video".to_string()));
    }
    let mut modalities = Map::new();
    modalities.insert("input".to_string(), Value::Array(input));
    modalities.insert(
        "output".to_string(),
        Value::Array(vec![Value::String("text".to_string())]),
    );
    modalities
}

/// Resolve the OpenCode npm package and base URL from the chosen compatibility
/// type: anthropic uses the Claude-compatible endpoint + @ai-sdk/anthropic,
/// everything else the OpenAI-compatible endpoint + @ai-sdk/openai-compatible.
/// Shared by apply_opencode and preview_for so they cannot drift.
fn opencode_npm_and_base_url(
    provider: &config::Provider,
    options: &config::OpencodeAgentModelOptions,
) -> (String, String) {
    if options.provider_type.trim() == "anthropic" {
        (
            "@ai-sdk/anthropic".to_string(),
            provider.endpoint_claude.clone(),
        )
    } else {
        (
            "@ai-sdk/openai-compatible".to_string(),
            provider.endpoint_openai.clone(),
        )
    }
}

fn apply_opencode(
    provider: &config::Provider,
    model: &config::Model,
    options: &config::OpencodeAgentModelOptions,
) -> Result<(), String> {
    let path = agent_config_path("opencode")?;
    let mut root = read_json_object(&path)?;
    let (npm, base_url) = opencode_npm_and_base_url(provider, options);
    root.insert(
        "$schema".to_string(),
        Value::String("https://opencode.ai/config.json".to_string()),
    );
    root.insert(
        "model".to_string(),
        Value::String(format!("{}/{}", provider.key, model.id)),
    );

    // Build the models object for the current provider from scratch so stale
    // entries can be removed afterwards.
    let mut new_models = Map::new();
    for m in &provider.models {
        let mut m_entry = Map::new();
        m_entry.insert(
            "name".to_string(),
            Value::String(if m.name.is_empty() {
                m.id.clone()
            } else {
                m.name.clone()
            }),
        );
        let mut limit = Map::new();
        if let Some(ctx) = m.context_window {
            limit.insert("context".to_string(), Value::Number(ctx.into()));
        }
        if let Some(max) = m.max_output {
            limit.insert("output".to_string(), Value::Number(max.into()));
        }
        if !limit.is_empty() {
            m_entry.insert("limit".to_string(), Value::Object(limit));
        }
        m_entry.insert(
            "modalities".to_string(),
            Value::Object(opencode_modalities(m)),
        );
        new_models.insert(m.id.clone(), Value::Object(m_entry));
    }

    // Replace the entire `provider` object with only the current provider key,
    // so stale provider entries from previous writes are removed.
    let mut opts = Map::new();
    opts.insert("baseURL".to_string(), Value::String(base_url));
    opts.insert(
        "apiKey".to_string(),
        Value::String(provider.api_key.clone()),
    );
    let mut entry = Map::new();
    entry.insert("name".to_string(), Value::String(provider.name.clone()));
    entry.insert("npm".to_string(), Value::String(npm));
    entry.insert("options".to_string(), Value::Object(opts));
    entry.insert("models".to_string(), Value::Object(new_models));
    let mut providers = Map::new();
    providers.insert(provider.key.clone(), Value::Object(entry));
    root.insert("provider".to_string(), Value::Object(providers));

    write_json_object(&path, root)
}

fn apply_codex(
    provider: &config::Provider,
    model: &config::Model,
    options: &config::CodexAgentModelOptions,
) -> Result<(), String> {
    let path = agent_config_path("codex")?;
    let mut doc = read_toml_doc(&path)?;
    let env_key = if options.env_key.trim().is_empty() {
        env_key_for_provider(&provider.key)
    } else {
        options.env_key.trim().to_string()
    };
    doc["model"] = toml_value(model.id.clone());
    doc["model_provider"] = toml_value(provider.key.clone());
    doc["model_providers"][&provider.key]["name"] = toml_value(provider.name.clone());
    doc["model_providers"][&provider.key]["base_url"] =
        toml_value(provider.endpoint_openai.clone());
    doc["model_providers"][&provider.key]["env_key"] = toml_value(env_key);
    // wire_api is fixed: only the "responses" wire is supported.
    doc["model_providers"][&provider.key]["wire_api"] = toml_value("responses");
    write_toml_doc(&path, doc)
}

fn apply_agent_model_config(agent: &str, saved: &config::AgentModelConfig) -> Result<(), String> {
    let cfg = config::load()?;
    let (provider, model) =
        validate_agent_ref(&cfg, agent, &saved.provider, &saved.model, &saved.options)?;
    match agent {
        "claude" => apply_claude(provider, model, &saved.options.claude),
        "kimi" => apply_kimi(provider, model, &saved.options.kimi),
        "opencode" => apply_opencode(provider, model, &saved.options.opencode),
        "codex" => apply_codex(provider, model, &saved.options.codex),
        _ => Err("未知 Agent".to_string()),
    }
}

fn preview_for(
    agent: &str,
    provider: &config::Provider,
    model: &config::Model,
    options: &config::AgentModelOptions,
) -> Result<Vec<AgentModelFileDto>, String> {
    let path = agent_config_path(agent)?.to_string_lossy().to_string();
    let files = match agent {
        "claude" => {
            // Build the same env map apply writes, then mask the API key for
            // display so the preview never leaks it.
            let mut env = build_claude_env(provider, model, &options.claude);
            env.insert(
                "ANTHROPIC_AUTH_TOKEN".to_string(),
                Value::String("<来自模型中心>".to_string()),
            );
            let mut settings = Map::new();
            settings.insert("env".to_string(), Value::Object(env));
            let settings_content = serde_json::to_string_pretty(&Value::Object(settings))
                .map_err(|e| format!("序列化失败: {e}"))?;

            // ~/.claude.json: always merge hasCompletedOnboarding: true.
            let mut onb = Map::new();
            onb.insert("hasCompletedOnboarding".to_string(), Value::Bool(true));
            let onb_content = serde_json::to_string_pretty(&Value::Object(onb))
                .map_err(|e| format!("序列化失败: {e}"))?;
            let onb_path = claude_onboarding_path()?.to_string_lossy().to_string();

            vec![
                AgentModelFileDto {
                    path,
                    content: settings_content,
                },
                AgentModelFileDto {
                    path: onb_path,
                    content: onb_content,
                },
            ]
        }
        "kimi" => {
            let ptype = options.kimi.provider_type.trim();
            let base_url = if ptype == "anthropic" {
                provider.endpoint_claude.clone()
            } else {
                provider.endpoint_openai.clone()
            };
            let caps: Vec<String> = options
                .kimi
                .capabilities
                .iter()
                .map(|c| c.trim())
                .filter(|c| !c.is_empty())
                .map(|c| format!("\"{c}\""))
                .collect();
            let caps_str = caps.join(", ");
            // default + every provider model, mirroring what apply writes.
            let mut content = format!(
                "default_model = \"{}/{}\"\n\n[providers.{}]\ntype = \"{}\"\nbase_url = \"{}\"\napi_key = \"<来自模型中心>\"\n",
                provider.key, model.id, provider.key, ptype, base_url
            );
            for m in &provider.models {
                let key = format!("{}/{}", provider.key, m.id);
                content.push_str(&format!(
                    "\n[models.\"{key}\"]\nprovider = \"{}\"\nmodel = \"{}\"\n",
                    provider.key, m.id
                ));
                if let Some(ctx) = m.context_window {
                    content.push_str(&format!("max_context_size = {ctx}\n"));
                }
                let display = if m.name.is_empty() {
                    m.id.clone()
                } else {
                    m.name.clone()
                };
                content.push_str(&format!(
                    "display_name = \"{display}\"\ncapabilities = [{caps_str}]\n"
                ));
            }
            vec![AgentModelFileDto { path, content }]
        }
        "opencode" => {
            let (npm, base_url) = opencode_npm_and_base_url(provider, &options.opencode);
            // Build the same structure apply writes, then mask the API key.
            let mut models_obj = Map::new();
            for m in &provider.models {
                let mut me = Map::new();
                me.insert(
                    "name".to_string(),
                    Value::String(if m.name.is_empty() {
                        m.id.clone()
                    } else {
                        m.name.clone()
                    }),
                );
                let mut limit = Map::new();
                if let Some(ctx) = m.context_window {
                    limit.insert("context".to_string(), Value::Number(ctx.into()));
                }
                if let Some(max) = m.max_output {
                    limit.insert("output".to_string(), Value::Number(max.into()));
                }
                if !limit.is_empty() {
                    me.insert("limit".to_string(), Value::Object(limit));
                }
                me.insert(
                    "modalities".to_string(),
                    Value::Object(opencode_modalities(m)),
                );
                models_obj.insert(m.id.clone(), Value::Object(me));
            }
            let mut entry = Map::new();
            entry.insert("name".to_string(), Value::String(provider.name.clone()));
            entry.insert("npm".to_string(), Value::String(npm));
            let mut opts = Map::new();
            opts.insert("baseURL".to_string(), Value::String(base_url));
            opts.insert(
                "apiKey".to_string(),
                Value::String("<来自模型中心>".to_string()),
            );
            entry.insert("options".to_string(), Value::Object(opts));
            entry.insert("models".to_string(), Value::Object(models_obj));
            let mut providers_obj = Map::new();
            providers_obj.insert(provider.key.clone(), Value::Object(entry));
            let mut root = Map::new();
            root.insert(
                "$schema".to_string(),
                Value::String("https://opencode.ai/config.json".to_string()),
            );
            root.insert(
                "model".to_string(),
                Value::String(format!("{}/{}", provider.key, model.id)),
            );
            root.insert("provider".to_string(), Value::Object(providers_obj));
            let content = serde_json::to_string_pretty(&Value::Object(root))
                .map_err(|e| format!("序列化失败: {e}"))?;
            vec![AgentModelFileDto { path, content }]
        }
        "codex" => {
            let env_key = if options.codex.env_key.trim().is_empty() {
                env_key_for_provider(&provider.key)
            } else {
                options.codex.env_key.clone()
            };
            vec![AgentModelFileDto {
                path,
                content: format!(
                    "model = \"{}\"\nmodel_provider = \"{}\"\n\n[model_providers.{}]\nname = \"{}\"\nbase_url = \"{}\"\nenv_key = \"{}\"\nwire_api = \"responses\"",
                    model.id, provider.key, provider.key, provider.name, provider.endpoint_openai, env_key
                ),
            }]
        }
        _ => return Err("未知 Agent".to_string()),
    };
    Ok(files)
}

fn agent_model_dto(agent: &str, cfg: &config::Config) -> Result<AgentModelConfigDto, String> {
    let (_, _, _, _, endpoint_kind) = model_agent_meta(agent).ok_or("未知 Agent")?;
    let saved = cfg.agent_models.get(agent).cloned().unwrap_or_default();
    let provider = cfg.providers.iter().find(|p| p.key == saved.provider);
    let model = provider.and_then(|p| p.models.iter().find(|m| m.id == saved.model));
    let mut error = String::new();
    if !saved.provider.is_empty() || !saved.model.is_empty() {
        if let Err(e) =
            validate_agent_ref(cfg, agent, &saved.provider, &saved.model, &saved.options)
        {
            error = e;
        }
    }
    Ok(AgentModelConfigDto {
        agent: agent_info_for_model(agent),
        config_path: agent_config_path(agent)?.to_string_lossy().to_string(),
        secondary_config_path: if agent == "claude" {
            claude_onboarding_path()?.to_string_lossy().to_string()
        } else {
            String::new()
        },
        endpoint_kind: endpoint_kind.to_string(),
        configured: !saved.provider.is_empty() && !saved.model.is_empty(),
        provider: saved.provider,
        model: saved.model,
        options: saved.options,
        resolved_provider: provider.map(provider_to_dto),
        resolved_model: model.map(model_to_dto),
        error,
    })
}

#[tauri::command]
pub fn list_agent_model_configs() -> Result<Vec<AgentModelConfigDto>, String> {
    let cfg = config::load()?;
    MODEL_AGENTS
        .iter()
        .map(|(id, _, _, _, _)| agent_model_dto(id, &cfg))
        .collect()
}

#[tauri::command]
pub fn preview_agent_model_config(
    agent: String,
    provider: String,
    model: String,
    options: AgentModelOptionsDto,
) -> Result<Vec<AgentModelFileDto>, String> {
    let cfg = config::load()?;
    let (p, m) = validate_agent_ref(&cfg, &agent, &provider, &model, &options)?;
    preview_for(&agent, p, m, &options)
}

#[tauri::command]
pub fn set_agent_model_config(
    agent: String,
    provider: String,
    model: String,
    options: AgentModelOptionsDto,
) -> Result<AgentModelConfigDto, String> {
    let mut cfg = config::load()?;
    validate_agent_ref(&cfg, &agent, &provider, &model, &options)?;
    let saved = config::AgentModelConfig {
        provider: provider.trim().to_string(),
        model: model.trim().to_string(),
        options,
    };
    cfg.agent_models.insert(agent.clone(), saved.clone());
    config::save(&cfg)?;
    apply_agent_model_config(&agent, &saved)?;
    let cfg = config::load()?;
    agent_model_dto(&agent, &cfg)
}

#[tauri::command]
pub fn sync_agent_model_config(agent: String) -> Result<AgentModelConfigDto, String> {
    let cfg = config::load()?;
    let saved = cfg
        .agent_models
        .get(&agent)
        .cloned()
        .ok_or("该 Agent 尚未配置模型")?;
    apply_agent_model_config(&agent, &saved)?;
    let cfg = config::load()?;
    agent_model_dto(&agent, &cfg)
}

#[tauri::command]
pub fn sync_all_agent_model_configs() -> Result<Vec<AgentModelSyncResult>, String> {
    let cfg = config::load()?;
    let mut out = Vec::new();
    for (agent, _, _, _, _) in MODEL_AGENTS {
        let Some(saved) = cfg.agent_models.get(*agent) else {
            continue;
        };
        match apply_agent_model_config(agent, saved) {
            Ok(()) => out.push(AgentModelSyncResult {
                agent: (*agent).to_string(),
                ok: true,
                message: "已同步".to_string(),
            }),
            Err(e) => out.push(AgentModelSyncResult {
                agent: (*agent).to_string(),
                ok: false,
                message: e,
            }),
        }
    }
    Ok(out)
}

// ─────────────────────────── model center ───────────────────────────

fn provider_to_dto(p: &config::Provider) -> ProviderDto {
    ProviderDto {
        key: p.key.clone(),
        name: p.name.clone(),
        endpoint_openai: p.endpoint_openai.clone(),
        endpoint_claude: p.endpoint_claude.clone(),
        site: p.site.clone(),
        color: p.color(),
        initials: p.initials(),
        has_key: !p.api_key.is_empty(),
        models: p
            .models
            .iter()
            .map(|m| ModelDto {
                id: m.id.clone(),
                name: m.name.clone(),
                context_window: m.context_window,
                max_output: m.max_output,
                capabilities: config::normalize_model_capabilities(&m.capabilities),
            })
            .collect(),
    }
}

#[tauri::command]
pub fn list_providers() -> Result<Vec<ProviderDto>, String> {
    let cfg = config::load()?;
    Ok(cfg.providers.iter().map(provider_to_dto).collect())
}

/// Create a provider. `api_key` of `None` means "no key" (leave unset); an
/// empty string is treated as unset too. Errors on duplicate key.
#[tauri::command]
pub fn create_provider(
    key: String,
    name: String,
    endpoint_openai: String,
    endpoint_claude: String,
    site: String,
    api_key: Option<String>,
) -> Result<ProviderDto, String> {
    let key = key.trim().to_string();
    config::validate_provider_key(&key)?;
    if name.trim().is_empty() {
        return Err("名称不能为空".to_string());
    }
    let mut cfg = config::load()?;
    if cfg.providers.iter().any(|p| p.key == key) {
        return Err("该英文标识已存在".to_string());
    }
    let provider = config::Provider {
        key,
        name: name.trim().to_string(),
        endpoint_openai: endpoint_openai.trim().to_string(),
        endpoint_claude: endpoint_claude.trim().to_string(),
        site: site.trim().to_string(),
        api_key: api_key
            .map(|k| k.trim().to_string())
            .filter(|k| !k.is_empty())
            .unwrap_or_default(),
        models: Vec::new(),
    };
    let dto = provider_to_dto(&provider);
    cfg.providers.push(provider);
    config::save(&cfg)?;
    Ok(dto)
}

/// Update a provider's editable fields. `api_key` follows the same rule as
/// `set_llm_config`: `None` keeps the stored key, `Some("")` clears it.
/// `key` is the current key; `new_key` may equal it.
#[tauri::command]
pub fn update_provider(
    key: String,
    new_key: String,
    name: String,
    endpoint_openai: String,
    endpoint_claude: String,
    site: String,
    api_key: Option<String>,
) -> Result<ProviderDto, String> {
    let new_key = new_key.trim().to_string();
    config::validate_provider_key(&new_key)?;
    if name.trim().is_empty() {
        return Err("名称不能为空".to_string());
    }
    let mut cfg = config::load()?;
    if new_key != key && cfg.providers.iter().any(|p| p.key == new_key) {
        return Err("该英文标识已存在".to_string());
    }
    let p = cfg
        .providers
        .iter_mut()
        .find(|p| p.key == key)
        .ok_or("Provider 不存在")?;
    p.key = new_key;
    p.name = name.trim().to_string();
    p.endpoint_openai = endpoint_openai.trim().to_string();
    p.endpoint_claude = endpoint_claude.trim().to_string();
    p.site = site.trim().to_string();
    if let Some(k) = api_key {
        p.api_key = k.trim().to_string();
    }
    let dto = provider_to_dto(p);
    config::save(&cfg)?;
    Ok(dto)
}

#[tauri::command]
pub fn delete_provider(key: String) -> Result<(), String> {
    let mut cfg = config::load()?;
    let before = cfg.providers.len();
    cfg.providers.retain(|p| p.key != key);
    if cfg.providers.len() == before {
        return Err("Provider 不存在".to_string());
    }
    config::save(&cfg)
}

#[tauri::command]
pub fn add_model(
    provider: String,
    id: String,
    name: String,
    context_window: Option<u64>,
    max_output: Option<u64>,
    capabilities: Vec<String>,
) -> Result<ProviderDto, String> {
    let id = id.trim().to_string();
    if id.is_empty() {
        return Err("模型 ID 不能为空".to_string());
    }
    let mut cfg = config::load()?;
    let p = cfg
        .providers
        .iter_mut()
        .find(|p| p.key == provider)
        .ok_or("Provider 不存在")?;
    if p.models.iter().any(|m| m.id == id) {
        return Err("该 provider 下已存在同名模型".to_string());
    }
    p.models.push(config::Model {
        id,
        name: name.trim().to_string(),
        // A value of 0 is treated as "unset".
        context_window: context_window.filter(|&v| v > 0),
        max_output: max_output.filter(|&v| v > 0),
        capabilities: config::normalize_model_capabilities(&capabilities),
    });
    let dto = provider_to_dto(p);
    config::save(&cfg)?;
    Ok(dto)
}

/// Edit a model's display name and optional limits. The model `id` (its key)
/// is not changed. Either limit may be `None` to clear it.
#[tauri::command]
pub fn update_model(
    provider: String,
    id: String,
    name: String,
    context_window: Option<u64>,
    max_output: Option<u64>,
    capabilities: Vec<String>,
) -> Result<ProviderDto, String> {
    let mut cfg = config::load()?;
    let p = cfg
        .providers
        .iter_mut()
        .find(|p| p.key == provider)
        .ok_or("Provider 不存在")?;
    let m = p
        .models
        .iter_mut()
        .find(|m| m.id == id)
        .ok_or("模型不存在")?;
    m.name = name.trim().to_string();
    m.context_window = context_window.filter(|&v| v > 0);
    m.max_output = max_output.filter(|&v| v > 0);
    m.capabilities = config::normalize_model_capabilities(&capabilities);
    let dto = provider_to_dto(p);
    config::save(&cfg)?;
    Ok(dto)
}

#[tauri::command]
pub fn remove_model(provider: String, id: String) -> Result<ProviderDto, String> {
    let mut cfg = config::load()?;
    let p = cfg
        .providers
        .iter_mut()
        .find(|p| p.key == provider)
        .ok_or("Provider 不存在")?;
    let before = p.models.len();
    p.models.retain(|m| m.id != id);
    if p.models.len() == before {
        return Err("模型不存在".to_string());
    }
    let dto = provider_to_dto(p);
    config::save(&cfg)?;
    Ok(dto)
}

// ─────────────────────────── translation ───────────────────────────

/// Return a cached Chinese translation for a skill's SKILL.md, or null when no
/// valid cache exists (source missing or changed since last translation).
#[tauri::command]
pub fn get_skill_translation(name: String) -> Result<Option<String>, String> {
    validate_name(&name)?;
    let md = paths::skill_path(&name)?.join("SKILL.md");
    let Ok(source) = std::fs::read_to_string(&md) else {
        return Ok(None);
    };
    let hash = config::content_hash(&source);
    Ok(config::read_cached_translation(&name, &hash))
}

/// Translate a skill's SKILL.md to Chinese via the configured OpenAI-compatible
/// endpoint, cache the result, and return it.
#[tauri::command]
pub async fn translate_skill(name: String) -> Result<TranslationDto, String> {
    validate_name(&name)?;
    let md = paths::skill_path(&name)?.join("SKILL.md");
    let source = std::fs::read_to_string(&md).map_err(|_| "该技能没有 SKILL.md".to_string())?;
    let hash = config::content_hash(&source);

    // Serve a still-valid cache without hitting the network.
    if let Some(cached) = config::read_cached_translation(&name, &hash) {
        return Ok(TranslationDto {
            text: cached,
            cached: true,
        });
    }

    let cfg = config::load()?.llm;
    if cfg.endpoint.is_empty() || cfg.model.is_empty() {
        return Err("尚未配置大语言模型，请先在「设置」中填写端点与模型".to_string());
    }
    if cfg.api_key.is_empty() {
        return Err("尚未配置 API 密钥，请先在「设置」中填写".to_string());
    }

    let translated = call_chat_completion(&cfg, &source).await?;
    config::write_cached_translation(&name, &hash, &translated)?;
    Ok(TranslationDto {
        text: translated,
        cached: false,
    })
}

/// Resolve the chat-completions URL from a user-entered endpoint, tolerating
/// either a base URL (`https://api.x.com/v1`) or the full path.
fn chat_completions_url(endpoint: &str) -> String {
    let base = endpoint.trim().trim_end_matches('/');
    if base.contains("/chat/completions") {
        base.to_string()
    } else {
        format!("{base}/chat/completions")
    }
}

async fn call_chat_completion(cfg: &config::LlmConfig, source: &str) -> Result<String, String> {
    let url = chat_completions_url(&cfg.endpoint);
    let system = "你是一名专业的技术文档译者。请把用户提供的 Markdown 文档忠实地翻译成简体中文，\
                  保留所有 Markdown 结构、代码块、行内代码、链接与 YAML frontmatter 的键名不变，\
                  只翻译说明性文字；代码、命令、路径、占位符保持原样。只输出翻译后的 Markdown，不要添加任何解释。";

    let body = serde_json::json!({
        "model": cfg.model,
        "temperature": 0.2,
        "messages": [
            { "role": "system", "content": system },
            { "role": "user", "content": source },
        ],
    });

    let client = reqwest::Client::new();
    let resp = client
        .post(&url)
        .bearer_auth(&cfg.api_key)
        .json(&body)
        .send()
        .await
        .map_err(|e| format!("请求失败: {e}"))?;

    let status = resp.status();
    let text = resp
        .text()
        .await
        .map_err(|e| format!("读取响应失败: {e}"))?;
    if !status.is_success() {
        return Err(format!(
            "翻译服务返回错误 {status}: {}",
            text.chars().take(300).collect::<String>()
        ));
    }

    let json: serde_json::Value =
        serde_json::from_str(&text).map_err(|e| format!("解析响应失败: {e}"))?;
    let content = json
        .get("choices")
        .and_then(|c| c.get(0))
        .and_then(|c| c.get("message"))
        .and_then(|m| m.get("content"))
        .and_then(|c| c.as_str())
        .ok_or_else(|| "响应中没有翻译内容".to_string())?;

    Ok(content.trim().to_string())
}

// ─────────────────────────── import ───────────────────────────

#[tauri::command]
pub async fn parse_github_import(
    link: String,
    branch: Option<String>,
) -> Result<ParsedImport, String> {
    import::parse_github(link, branch).await
}

#[tauri::command]
pub async fn parse_url_import(url: String) -> Result<ParsedImport, String> {
    import::parse_url(url).await
}

#[tauri::command]
pub fn parse_local_import(path: String) -> Result<ParsedImport, String> {
    import::parse_local(path)
}

#[tauri::command]
pub fn import_from_staging(
    root: String,
    is_temp: bool,
    selections: Vec<ImportSelection>,
) -> Result<ImportResult, String> {
    import::import_selected(root, is_temp, selections)
}

#[tauri::command]
pub fn cancel_import(root: String, is_temp: bool) {
    import::cancel(root, is_temp);
}

/// Create a skill directly from pasted content (or a generated template when
/// the content is empty). Replaces the former "新建技能" flow.
#[tauri::command]
pub fn create_skill_text(name: String, content: String) -> Result<(), String> {
    validate_name(&name)?;
    paths::ensure_hub()?;
    let dir = paths::skill_path(&name)?;
    if dir.exists() {
        return Err("技能已存在".to_string());
    }
    std::fs::create_dir_all(&dir).map_err(|e| format!("创建技能失败: {e}"))?;
    let md = if content.trim().is_empty() {
        format!("---\nname: {name}\ndescription: 描述这个技能的用途。\n---\n\n# {name}\n")
    } else {
        content
    };
    std::fs::write(dir.join("SKILL.md"), md).map_err(|e| format!("写入 SKILL.md 失败: {e}"))
}

// ─────────────────────────── misc ───────────────────────────

#[tauri::command]
pub fn open_path(path: String) -> Result<(), String> {
    // Expand a leading `~/` to the user's home directory.
    let expanded = if let Some(rest) = path.strip_prefix("~/") {
        paths::home_dir()?.join(rest).to_string_lossy().to_string()
    } else {
        path
    };
    let mut cmd = if cfg!(target_os = "macos") {
        let mut c = std::process::Command::new("open");
        c.arg(&expanded);
        c
    } else if cfg!(target_os = "windows") {
        let mut c = std::process::Command::new("explorer");
        c.arg(&expanded);
        c
    } else {
        let mut c = std::process::Command::new("xdg-open");
        c.arg(&expanded);
        c
    };
    cmd.spawn().map_err(|e| format!("打开失败: {e}"))?;
    Ok(())
}
