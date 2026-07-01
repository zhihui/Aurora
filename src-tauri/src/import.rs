use crate::meta;
use crate::paths;
use serde::{Deserialize, Serialize};
use std::io::{Read, Seek};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

// ─────────────────────────── DTOs ───────────────────────────

#[derive(Serialize)]
pub struct DetectedSkill {
    /// Path relative to the staging root; "" means the root itself is a skill.
    pub rel_path: String,
    /// Suggested skill name (frontmatter `name:`, else directory name).
    pub name: String,
    pub description: String,
    /// True when a center skill with this name already exists.
    pub exists: bool,
}

#[derive(Serialize)]
pub struct ParsedImport {
    /// Absolute path the rel_paths are based on (and what import copies from).
    pub root: String,
    /// Whether `root` lives in the temp staging area and may be cleaned up.
    pub is_temp: bool,
    pub skills: Vec<DetectedSkill>,
}

#[derive(Deserialize)]
pub struct ImportSelection {
    pub rel_path: String,
    /// Final name to use in the center (allows rename on collision).
    pub name: String,
}

#[derive(Serialize)]
pub struct ImportResult {
    pub imported: Vec<String>,
    pub errors: Vec<String>,
}

// ─────────────────────────── shared helpers ───────────────────────────

fn validate_name(name: &str) -> Result<(), String> {
    if name.is_empty() || name.contains('/') || name.contains('\\') || name.contains("..") {
        return Err(format!("名称不合法: {name}"));
    }
    Ok(())
}

fn copy_dir_all(src: &Path, dst: &Path) -> Result<(), String> {
    std::fs::create_dir_all(dst).map_err(|e| format!("创建目录失败: {e}"))?;
    for entry in std::fs::read_dir(src).map_err(|e| format!("读取目录失败: {e}"))? {
        let entry = entry.map_err(|e| e.to_string())?;
        let from = entry.path();
        let to = dst.join(entry.file_name());
        if entry.file_type().map_err(|e| e.to_string())?.is_dir() {
            copy_dir_all(&from, &to)?;
        } else {
            std::fs::copy(&from, &to).map_err(|e| format!("复制文件失败: {e}"))?;
        }
    }
    Ok(())
}

/// Create a fresh staging directory: ~/.aurora/cache/import/<id>
fn new_staging_dir() -> Result<PathBuf, String> {
    static COUNTER: AtomicU64 = AtomicU64::new(0);
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    let n = COUNTER.fetch_add(1, Ordering::Relaxed);
    let id = format!("{nanos:x}-{n:x}");
    let dir = paths::hub_root()?.join("cache").join("import").join(id);
    std::fs::create_dir_all(&dir).map_err(|e| format!("创建暂存目录失败: {e}"))?;
    Ok(dir)
}

/// The `<id>` directory under cache/import that contains `root`, if any.
/// Guards cleanup so we never remove anything outside the staging area.
fn temp_id_dir(root: &Path) -> Option<PathBuf> {
    let base = paths::hub_root().ok()?.join("cache").join("import");
    let rel = root.strip_prefix(&base).ok()?;
    let first = rel.components().next()?;
    Some(base.join(first.as_os_str()))
}

fn is_hidden(name: &std::ffi::OsStr) -> bool {
    name.to_string_lossy().starts_with('.')
}

/// Recursively find directories that directly contain a SKILL.md. A directory
/// that is itself a skill is recorded and not descended into.
fn detect_skills(scan_root: &Path, fallback_root_name: &str) -> Vec<DetectedSkill> {
    let mut out = Vec::new();
    walk(scan_root, scan_root, fallback_root_name, 0, &mut out);
    out.sort_by(|a, b| a.name.to_lowercase().cmp(&b.name.to_lowercase()));
    out
}

fn walk(
    base: &Path,
    dir: &Path,
    fallback_root_name: &str,
    depth: usize,
    out: &mut Vec<DetectedSkill>,
) {
    if depth > 6 {
        return;
    }
    let skill_md = dir.join("SKILL.md");
    if skill_md.is_file() {
        let rel = dir.strip_prefix(base).unwrap_or(Path::new(""));
        let rel_path = rel.to_string_lossy().to_string();
        let name = meta::read_name(&skill_md).unwrap_or_else(|| {
            if rel_path.is_empty() {
                fallback_root_name.to_string()
            } else {
                dir.file_name()
                    .map(|n| n.to_string_lossy().to_string())
                    .unwrap_or_default()
            }
        });
        let exists = paths::skill_path(&name)
            .map(|p| p.exists())
            .unwrap_or(false);
        out.push(DetectedSkill {
            rel_path,
            description: meta::read_description(dir),
            name,
            exists,
        });
        return;
    }
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for entry in entries.filter_map(|e| e.ok()) {
        if is_hidden(&entry.file_name()) {
            continue;
        }
        let p = entry.path();
        if p.is_dir() {
            walk(base, &p, fallback_root_name, depth + 1, out);
        }
    }
}

// ─────────────────────────── extraction ───────────────────────────

fn extract_zip<R: Read + Seek>(reader: R, dest: &Path) -> Result<(), String> {
    let mut zip = zip::ZipArchive::new(reader).map_err(|e| format!("打开 zip 失败: {e}"))?;
    for i in 0..zip.len() {
        let mut file = zip
            .by_index(i)
            .map_err(|e| format!("读取 zip 条目失败: {e}"))?;
        // enclosed_name() rejects absolute paths and `..` traversal (zip-slip).
        let Some(rel) = file.enclosed_name() else {
            continue;
        };
        let out_path = dest.join(rel);
        if file.is_dir() {
            std::fs::create_dir_all(&out_path).map_err(|e| format!("创建目录失败: {e}"))?;
        } else {
            if let Some(parent) = out_path.parent() {
                std::fs::create_dir_all(parent).map_err(|e| format!("创建目录失败: {e}"))?;
            }
            let mut out = std::fs::File::create(&out_path).map_err(|e| format!("写入失败: {e}"))?;
            std::io::copy(&mut file, &mut out).map_err(|e| format!("解压失败: {e}"))?;
        }
    }
    Ok(())
}

fn extract_tar_gz<R: Read>(reader: R, dest: &Path) -> Result<(), String> {
    let gz = flate2::read::GzDecoder::new(reader);
    let mut archive = tar::Archive::new(gz);
    std::fs::create_dir_all(dest).map_err(|e| format!("创建目录失败: {e}"))?;
    for entry in archive
        .entries()
        .map_err(|e| format!("读取归档失败: {e}"))?
    {
        let mut entry = entry.map_err(|e| format!("读取条目失败: {e}"))?;
        // unpack_in guards against paths escaping `dest`.
        entry
            .unpack_in(dest)
            .map_err(|e| format!("解压失败: {e}"))?;
    }
    Ok(())
}

/// After extracting an archive into `dir`, descend through single wrapper
/// directories (e.g. the `repo-branch/` GitHub adds) to the real content root.
fn unwrap_single_dir(dir: &Path) -> PathBuf {
    let mut current = dir.to_path_buf();
    loop {
        let Ok(rd) = std::fs::read_dir(&current) else {
            break;
        };
        let entries: Vec<_> = rd
            .filter_map(|e| e.ok())
            .filter(|e| !is_hidden(&e.file_name()))
            .collect();
        // Only unwrap when the sole entry is a directory AND it isn't itself a skill.
        if entries.len() == 1 && entries[0].path().is_dir() {
            let only = entries[0].path();
            if only.join("SKILL.md").is_file() {
                break;
            }
            current = only;
        } else {
            break;
        }
    }
    current
}

// ─────────────────────────── GitHub ───────────────────────────

struct GhRef {
    owner: String,
    repo: String,
    branch: Option<String>,
    subpath: String,
}

fn parse_github_ref(link: &str, branch: Option<String>) -> Result<GhRef, String> {
    let s = link.trim();
    // git@github.com:owner/repo(.git)
    let s = s
        .strip_prefix("git@github.com:")
        .map(|r| r.to_string())
        .unwrap_or_else(|| {
            s.strip_prefix("https://github.com/")
                .or_else(|| s.strip_prefix("http://github.com/"))
                .or_else(|| s.strip_prefix("github.com/"))
                .unwrap_or(s)
                .to_string()
        });
    let s = s.trim_end_matches('/');
    let parts: Vec<&str> = s.split('/').filter(|p| !p.is_empty()).collect();
    if parts.len() < 2 {
        return Err("无法识别的 GitHub 链接".to_string());
    }
    let owner = parts[0].to_string();
    let repo = parts[1].trim_end_matches(".git").to_string();
    let mut branch = branch.filter(|b| !b.trim().is_empty());
    let mut subpath = String::new();
    // .../tree/<branch>/<subpath...>  or  .../blob/<branch>/<subpath...>
    if parts.len() >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
        if branch.is_none() {
            branch = Some(parts[3].to_string());
        }
        subpath = parts[4..].join("/");
    }
    Ok(GhRef {
        owner,
        repo,
        branch,
        subpath,
    })
}

pub async fn parse_github(link: String, branch: Option<String>) -> Result<ParsedImport, String> {
    let gh = parse_github_ref(&link, branch)?;
    let candidates: Vec<String> = match &gh.branch {
        Some(b) => vec![b.clone()],
        None => vec!["main".to_string(), "master".to_string()],
    };

    let client = reqwest::Client::builder()
        .user_agent("aurora")
        .build()
        .map_err(|e| format!("初始化失败: {e}"))?;

    let mut bytes: Option<Vec<u8>> = None;
    let mut last_err = String::new();
    for b in &candidates {
        let url = format!(
            "https://codeload.github.com/{}/{}/tar.gz/refs/heads/{}",
            gh.owner, gh.repo, b
        );
        match client.get(&url).send().await {
            Ok(resp) if resp.status().is_success() => {
                let body = resp.bytes().await.map_err(|e| format!("下载失败: {e}"))?;
                bytes = Some(body.to_vec());
                break;
            }
            Ok(resp) => last_err = format!("分支 {b} 返回 {}", resp.status()),
            Err(e) => last_err = format!("请求失败: {e}"),
        }
    }
    let bytes = bytes.ok_or_else(|| format!("下载仓库归档失败：{last_err}"))?;

    let staging = new_staging_dir()?;
    extract_tar_gz(std::io::Cursor::new(&bytes[..]), &staging)?;

    // GitHub archives wrap everything in a single `repo-branch/` folder.
    let top = unwrap_single_dir(&staging);
    let scan_root = if gh.subpath.is_empty() {
        top.clone()
    } else {
        top.join(&gh.subpath)
    };
    if !scan_root.is_dir() {
        cleanup(&staging);
        return Err("仓库中找不到该子目录".to_string());
    }

    let fallback = if gh.subpath.is_empty() {
        gh.repo.clone()
    } else {
        gh.subpath
            .rsplit('/')
            .next()
            .unwrap_or(&gh.repo)
            .to_string()
    };
    let skills = detect_skills(&scan_root, &fallback);
    if skills.is_empty() {
        cleanup(&staging);
        return Err("该位置下没有找到包含 SKILL.md 的技能".to_string());
    }
    Ok(ParsedImport {
        root: scan_root.to_string_lossy().to_string(),
        is_temp: true,
        skills,
    })
}

// ─────────────────────────── arbitrary URL ───────────────────────────

pub async fn parse_url(url: String) -> Result<ParsedImport, String> {
    let url = url.trim().to_string();
    if !(url.starts_with("http://") || url.starts_with("https://")) {
        return Err("请输入 http(s) 链接".to_string());
    }
    let lower = url.to_lowercase();
    let client = reqwest::Client::builder()
        .user_agent("aurora")
        .build()
        .map_err(|e| format!("初始化失败: {e}"))?;
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| format!("请求失败: {e}"))?;
    if !resp.status().is_success() {
        return Err(format!("下载失败：{}", resp.status()));
    }
    let staging = new_staging_dir()?;

    if lower.ends_with(".zip") {
        let bytes = resp.bytes().await.map_err(|e| format!("下载失败: {e}"))?;
        extract_zip(std::io::Cursor::new(&bytes[..]), &staging)?;
    } else if lower.ends_with(".tar.gz") || lower.ends_with(".tgz") {
        let bytes = resp.bytes().await.map_err(|e| format!("下载失败: {e}"))?;
        extract_tar_gz(std::io::Cursor::new(&bytes[..]), &staging)?;
    } else {
        // Treat as a single SKILL.md document.
        let text = resp.text().await.map_err(|e| format!("下载失败: {e}"))?;
        // Infer a name from frontmatter, else the URL filename stem.
        let name = frontmatter_name_from_str(&text).unwrap_or_else(|| {
            url.rsplit('/')
                .find(|s| !s.is_empty())
                .map(|f| {
                    f.trim_end_matches(".md")
                        .trim_end_matches(".markdown")
                        .to_string()
                })
                .filter(|s| !s.is_empty() && s.to_lowercase() != "skill")
                .unwrap_or_else(|| "imported-skill".to_string())
        });
        let dir = staging.join(&name);
        std::fs::create_dir_all(&dir).map_err(|e| format!("创建目录失败: {e}"))?;
        std::fs::write(dir.join("SKILL.md"), text).map_err(|e| format!("写入失败: {e}"))?;
    }

    let scan_root = unwrap_single_dir(&staging);
    let skills = detect_skills(&scan_root, "imported-skill");
    if skills.is_empty() {
        cleanup(&staging);
        return Err("下载内容中没有找到包含 SKILL.md 的技能".to_string());
    }
    Ok(ParsedImport {
        root: scan_root.to_string_lossy().to_string(),
        is_temp: true,
        skills,
    })
}

fn frontmatter_name_from_str(text: &str) -> Option<String> {
    let rest = text.trim_start().strip_prefix("---")?;
    let end = rest.find("\n---")?;
    for line in rest[..end].lines() {
        if let Some(v) = line.trim().strip_prefix("name:") {
            let v = v.trim().trim_matches(|c| c == '"' || c == '\'').trim();
            if !v.is_empty() {
                return Some(v.to_string());
            }
        }
    }
    None
}

// ─────────────────────────── local ───────────────────────────

pub fn parse_local(path: String) -> Result<ParsedImport, String> {
    let src = PathBuf::from(&path);
    if !src.exists() {
        return Err("路径不存在".to_string());
    }
    let lower = path.to_lowercase();

    if src.is_dir() {
        let fallback = src
            .file_name()
            .map(|n| n.to_string_lossy().to_string())
            .unwrap_or_else(|| "imported-skill".to_string());
        let skills = detect_skills(&src, &fallback);
        if skills.is_empty() {
            return Err("该目录下没有找到包含 SKILL.md 的技能".to_string());
        }
        return Ok(ParsedImport {
            root: src.to_string_lossy().to_string(),
            is_temp: false,
            skills,
        });
    }

    // Archive file → extract into staging.
    let staging = new_staging_dir()?;
    if lower.ends_with(".zip") {
        let file = std::fs::File::open(&src).map_err(|e| format!("打开文件失败: {e}"))?;
        extract_zip(file, &staging)?;
    } else if lower.ends_with(".tar.gz") || lower.ends_with(".tgz") {
        let file = std::fs::File::open(&src).map_err(|e| format!("打开文件失败: {e}"))?;
        extract_tar_gz(file, &staging)?;
    } else {
        cleanup(&staging);
        return Err("仅支持文件夹、.zip 或 .tar.gz".to_string());
    }

    let scan_root = unwrap_single_dir(&staging);
    let fallback = src
        .file_stem()
        .map(|n| n.to_string_lossy().trim_end_matches(".tar").to_string())
        .unwrap_or_else(|| "imported-skill".to_string());
    let skills = detect_skills(&scan_root, &fallback);
    if skills.is_empty() {
        cleanup(&staging);
        return Err("压缩包中没有找到包含 SKILL.md 的技能".to_string());
    }
    Ok(ParsedImport {
        root: scan_root.to_string_lossy().to_string(),
        is_temp: true,
        skills,
    })
}

// ─────────────────────────── import / cancel ───────────────────────────

pub fn import_selected(
    root: String,
    is_temp: bool,
    selections: Vec<ImportSelection>,
) -> Result<ImportResult, String> {
    paths::ensure_hub()?;
    let root = PathBuf::from(&root);
    let mut imported = Vec::new();
    let mut errors = Vec::new();

    for sel in &selections {
        let name = sel.name.trim();
        if let Err(e) = validate_name(name) {
            errors.push(e);
            continue;
        }
        // rel_path is staging-relative; reject traversal.
        if sel.rel_path.contains("..") {
            errors.push(format!("{name}：非法路径"));
            continue;
        }
        let src = if sel.rel_path.is_empty() {
            root.clone()
        } else {
            root.join(&sel.rel_path)
        };
        if !src.join("SKILL.md").is_file() {
            errors.push(format!("{name}：来源缺少 SKILL.md"));
            continue;
        }
        let dest = match paths::skill_path(name) {
            Ok(d) => d,
            Err(e) => {
                errors.push(format!("{name}：{e}"));
                continue;
            }
        };
        if dest.exists() {
            errors.push(format!("{name}：技能中心已存在同名技能"));
            continue;
        }
        match copy_dir_all(&src, &dest) {
            Ok(()) => imported.push(name.to_string()),
            Err(e) => errors.push(format!("{name}：{e}")),
        }
    }

    if is_temp {
        if let Some(id_dir) = temp_id_dir(&root) {
            cleanup(&id_dir);
        }
    }
    Ok(ImportResult { imported, errors })
}

pub fn cancel(root: String, is_temp: bool) {
    if !is_temp {
        return;
    }
    if let Some(id_dir) = temp_id_dir(&PathBuf::from(&root)) {
        cleanup(&id_dir);
    }
}

fn cleanup(dir: &Path) {
    let _ = std::fs::remove_dir_all(dir);
}
