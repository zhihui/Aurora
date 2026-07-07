use crate::config;
use std::path::{Path, PathBuf};

/// A supported agent and the absolute path to its skills directory.
/// Owned type: built-in entries are cloned from the static tables, user-added
/// entries come from config.json.
#[derive(Debug, Clone)]
pub struct Agent {
    pub id: String,
    pub name: String,
    /// CSS color used by the UI for this agent's badge.
    pub color: String,
    /// Path relative to the user's home directory, e.g. ".claude/skills".
    pub rel_dir: String,
}

/// Static descriptor used by both the built-in table and the candidate table.
pub struct AgentDef {
    pub id: &'static str,
    pub name: &'static str,
    pub color: &'static str,
    pub rel_dir: &'static str,
}

/// Built-in agents, always present unless the user explicitly removed them
/// (only `kimi` is removable; the core 4 are not). Order is the display order.
const BUILTIN_AGENTS: &[AgentDef] = &[
    AgentDef {
        id: "claude",
        name: "Claude Code",
        color: "#D97757",
        rel_dir: ".claude/skills",
    },
    AgentDef {
        id: "codex",
        name: "Codex",
        color: "#10A37F",
        rel_dir: ".codex/skills",
    },
    AgentDef {
        id: "opencode",
        name: "Opencode",
        color: "#EAB308",
        rel_dir: ".config/opencode/skills",
    },
    AgentDef {
        id: "agents",
        name: "通用 Agent",
        color: "#64748B",
        rel_dir: ".agents/skills",
    },
    AgentDef {
        id: "kimi",
        name: "Kimi Code",
        color: "#6366F1",
        rel_dir: ".kimi-code/skills",
    },
];

/// Agents the user may add from the Settings page. Each carries a fixed
/// skills directory under the home folder.
pub const CANDIDATE_AGENTS: &[AgentDef] = &[
    AgentDef {
        id: "codebuddy",
        name: "CodeBuddy",
        color: "#3B82F6",
        rel_dir: ".codebuddy/skills",
    },
    AgentDef {
        id: "hermes",
        name: "Hermes Agent",
        color: "#8B5CF6",
        rel_dir: ".hermes/skills",
    },
    AgentDef {
        id: "qoder",
        name: "Qoder",
        color: "#EC4899",
        rel_dir: ".qoder/skills",
    },
    AgentDef {
        id: "qwen",
        name: "Qwen Code",
        color: "#0EA5E9",
        rel_dir: ".qwen/skills",
    },
    AgentDef {
        id: "trae",
        name: "Trae",
        color: "#F97316",
        rel_dir: ".trae/skills",
    },
    AgentDef {
        id: "workbuddy",
        name: "Workbuddy",
        color: "#14B8A6",
        rel_dir: ".workbuddy/skills",
    },
];

/// The 4 core built-in agent ids that can never be removed.
const NON_REMOVABLE: &[&str] = &["claude", "codex", "opencode", "agents"];

fn agent_def_to_agent(d: &AgentDef) -> Agent {
    Agent {
        id: d.id.to_string(),
        name: d.name.to_string(),
        color: d.color.to_string(),
        rel_dir: d.rel_dir.to_string(),
    }
}

/// True for built-in agents the user is allowed to remove (kimi), false for the
/// core 4. Custom (user-added) agents are always removable.
pub fn is_removable(id: &str) -> bool {
    !NON_REMOVABLE.contains(&id)
}

/// Look up a candidate definition by id.
pub fn candidate_by_id(id: &str) -> Option<&'static AgentDef> {
    CANDIDATE_AGENTS.iter().find(|c| c.id == id)
}

/// Load the full active agent list: built-ins (minus any the user removed) plus
/// user-added custom agents. Built-ins come first, in display order.
pub fn load_agents() -> Vec<Agent> {
    let cfg = config::load().unwrap_or_default();
    let removed: Vec<&str> = cfg.removed_builtin.iter().map(|s| s.as_str()).collect();
    let mut out: Vec<Agent> = BUILTIN_AGENTS
        .iter()
        .filter(|d| !removed.contains(&d.id))
        .map(agent_def_to_agent)
        .collect();
    for ca in &cfg.custom_agents {
        // Skip custom agents that collide with a built-in id already present.
        if out.iter().any(|a| a.id == ca.id) {
            continue;
        }
        out.push(Agent {
            id: ca.id.clone(),
            name: ca.name.clone(),
            color: ca.color.clone(),
            rel_dir: ca.rel_dir.clone(),
        });
    }
    out
}

/// Find a single agent by id from the active list.
pub fn find_agent(id: &str) -> Option<Agent> {
    load_agents().into_iter().find(|a| a.id == id)
}

pub fn home_dir() -> Result<PathBuf, String> {
    dirs::home_dir().ok_or_else(|| "无法定位用户主目录".to_string())
}

impl Agent {
    pub fn skills_dir(&self) -> Result<PathBuf, String> {
        // `rel_dir` uses forward slashes; on Windows `PathBuf::join` accepts
        // them but `to_string_lossy()` may preserve the `/`. Normalize to the
        // platform's native separator so displayed paths and `explorer` args
        // are Windows-style (`C:\Users\...\.claude\skills`).
        let joined = home_dir()?.join(&self.rel_dir);
        Ok(native_path(&joined))
    }
}

/// Render a path with the platform's native separator. On Windows this turns
/// any `/` into `\`; elsewhere it leaves the path as-is.
fn native_path(p: &Path) -> PathBuf {
    if cfg!(target_os = "windows") {
        PathBuf::from(p.to_string_lossy().replace('/', "\\"))
    } else {
        p.to_path_buf()
    }
}

/// Root of Aurora data: ~/.aurora
pub fn hub_root() -> Result<PathBuf, String> {
    Ok(home_dir()?.join(".aurora"))
}

/// Skill center directory: ~/.aurora/skills
pub fn skills_dir() -> Result<PathBuf, String> {
    Ok(hub_root()?.join("skills"))
}

/// Absolute path to a center skill: ~/.aurora/skills/<name>
pub fn skill_path(name: &str) -> Result<PathBuf, String> {
    Ok(skills_dir()?.join(name))
}

/// packs.json path
pub fn packs_file() -> Result<PathBuf, String> {
    Ok(hub_root()?.join("packs.json"))
}

/// config.json path
pub fn config_file() -> Result<PathBuf, String> {
    Ok(hub_root()?.join("config.json"))
}

/// Cache directory: ~/.aurora/cache (created if missing).
pub fn cache_dir() -> Result<PathBuf, String> {
    let d = hub_root()?.join("cache");
    std::fs::create_dir_all(&d).map_err(|e| format!("创建缓存目录失败: {e}"))?;
    Ok(d)
}

/// Recreate a directory link (symlink on Unix, junction on Windows) at `link`
/// pointing to `target`.
fn make_dir_link(target: &Path, link: &Path) -> std::io::Result<()> {
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
fn remove_dir_link(link: &Path) -> std::io::Result<()> {
    #[cfg(unix)]
    {
        std::fs::remove_file(link)
    }
    #[cfg(windows)]
    {
        junction::delete(link)
    }
}

/// One-time migration from the legacy `~/.skills-hub` data root to `~/.aurora`.
///
/// Renames the directory if only the legacy one exists, then rewrites every
/// agent skill link whose target still points inside the old root so existing
/// assignments keep resolving. External / real-dir entries are left untouched.
fn migrate_hub() {
    let Ok(home) = home_dir() else { return };
    migrate_hub_in(&home);
}

fn migrate_hub_in(home: &Path) {
    let new_root = home.join(".aurora");
    let old_root = home.join(".skills-hub");

    // Move the data directory if the new root is missing but the old one exists.
    if !new_root.exists() && old_root.exists() {
        let _ = std::fs::rename(&old_root, &new_root);
    }

    // Rewrite agent links that still point into the old root.
    for agent in load_agents() {
        let dir = home.join(&agent.rel_dir);
        let entries = match std::fs::read_dir(&dir) {
            Ok(e) => e,
            Err(_) => continue,
        };
        for entry in entries.flatten() {
            let path = entry.path();
            let Some(target) = read_any_link(&path) else {
                continue;
            };
            // Only absolute targets pointing into the old root need rewriting.
            if !target.is_absolute() {
                continue;
            }
            let Some(rel) = target.strip_prefix(&old_root).ok() else {
                continue;
            };
            let new_target = new_root.join(rel);
            if read_any_link(&path).as_ref() == Some(&new_target) {
                continue;
            }
            if remove_dir_link(&path).is_ok() {
                let _ = make_dir_link(&new_target, &path);
            }
        }
    }
}

/// Ensure the hub data directories exist.
pub fn ensure_hub() -> Result<(), String> {
    migrate_hub();
    let skills = skills_dir()?;
    std::fs::create_dir_all(&skills).map_err(|e| format!("创建数据目录失败: {e}"))?;
    Ok(())
}

/// If `link` is a symlink (any OS) or a Windows junction, return its raw target.
pub fn read_any_link(link: &Path) -> Option<PathBuf> {
    let meta = std::fs::symlink_metadata(link).ok()?;
    if meta.file_type().is_symlink() {
        return std::fs::read_link(link).ok();
    }
    #[cfg(windows)]
    {
        if junction::exists(link).unwrap_or(false) {
            return junction::get_target(link).ok();
        }
    }
    None
}

/// True if `link` is a symlink/junction whose target resolves to `expected`.
pub fn symlink_points_to(link: &Path, expected: &Path) -> bool {
    let Some(target) = read_any_link(link) else {
        return false;
    };
    let resolved = if target.is_absolute() {
        target
    } else {
        link.parent().map(|p| p.join(&target)).unwrap_or(target)
    };
    // Compare canonicalized paths when possible, else compare normalized.
    match (resolved.canonicalize(), expected.canonicalize()) {
        (Ok(a), Ok(b)) => a == b,
        _ => resolved == *expected,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Build a temp home with legacy data + agent links, run the migration.
    fn setup_and_migrate() -> (tempfile::TempDir, PathBuf) {
        let tmp = tempfile::tempdir().expect("tmp");
        let home = tmp.path().to_path_buf();
        let old = home.join(".skills-hub");
        let new = home.join(".aurora");

        // Legacy data: a real skill dir + a config file.
        std::fs::create_dir_all(old.join("skills").join("demo")).unwrap();
        std::fs::write(old.join("config.json"), "{}").unwrap();

        // An agent skills dir holding a symlink into the old root, plus an
        // external symlink that must be left untouched.
        let agent_dir = home.join(".claude").join("skills");
        std::fs::create_dir_all(&agent_dir).unwrap();
        std::os::unix::fs::symlink(old.join("skills").join("demo"), agent_dir.join("demo"))
            .unwrap();
        let external = home.join("elsewhere");
        std::fs::create_dir_all(&external).unwrap();
        std::os::unix::fs::symlink(&external, agent_dir.join("ext")).unwrap();

        migrate_hub_in(&home);
        (tmp, new)
    }

    #[test]
    fn migrates_dir_and_rewrites_links() {
        let (_tmp, new) = setup_and_migrate();
        let home = new.parent().unwrap();

        // Directory moved.
        assert!(new.exists(), "new root should exist");
        assert!(
            !home.join(".skills-hub").exists(),
            "old root should be gone"
        );
        assert!(new.join("skills").join("demo").is_dir());
        assert!(new.join("config.json").exists());

        // Center symlink rewritten to the new root.
        let link = home.join(".claude").join("skills").join("demo");
        let target = read_any_link(&link).expect("link still present");
        assert_eq!(target, new.join("skills").join("demo"));
        assert!(symlink_points_to(&link, &new.join("skills").join("demo")));

        // External link untouched.
        let ext = home.join(".claude").join("skills").join("ext");
        assert_eq!(read_any_link(&ext), Some(home.join("elsewhere")));
    }

    #[test]
    fn migrate_is_idempotent() {
        let (_tmp, new) = setup_and_migrate();
        let home = new.parent().unwrap();
        let before = read_any_link(&home.join(".claude").join("skills").join("demo"));
        migrate_hub_in(&home); // second run must not churn
        let after = read_any_link(&home.join(".claude").join("skills").join("demo"));
        assert_eq!(before, after);
        assert!(home.join(".aurora").exists());
    }
}
