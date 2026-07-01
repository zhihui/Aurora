use std::path::Path;

/// Read the human description for a skill directory by parsing its SKILL.md.
/// Prefers a YAML frontmatter `description:` field, then the first non-heading
/// line of prose. Returns an empty string when nothing is found.
pub fn read_description(skill_dir: &Path) -> String {
    let md = skill_dir.join("SKILL.md");
    let Ok(content) = std::fs::read_to_string(&md) else {
        return String::new();
    };

    let trimmed = content.trim_start();
    if let Some(rest) = trimmed.strip_prefix("---") {
        // Frontmatter block: read until the closing `---`.
        if let Some(end) = rest.find("\n---") {
            let front = &rest[..end];
            for line in front.lines() {
                let line = line.trim();
                if let Some(v) = line.strip_prefix("description:") {
                    return clean_value(v);
                }
            }
        }
    }

    // Fall back to the first meaningful prose line.
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') || line.starts_with("---") {
            continue;
        }
        return line.to_string();
    }
    String::new()
}

fn clean_value(v: &str) -> String {
    let v = v.trim();
    let v = v.trim_matches(|c| c == '"' || c == '\'');
    v.trim().to_string()
}

/// Read the `name:` field from a SKILL.md's YAML frontmatter, if present.
pub fn read_name(skill_md: &Path) -> Option<String> {
    let content = std::fs::read_to_string(skill_md).ok()?;
    let trimmed = content.trim_start();
    let rest = trimmed.strip_prefix("---")?;
    let end = rest.find("\n---")?;
    for line in rest[..end].lines() {
        if let Some(v) = line.trim().strip_prefix("name:") {
            let v = clean_value(v);
            if !v.is_empty() {
                return Some(v);
            }
        }
    }
    None
}
