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
            if let Some(v) = front_description(&rest[..end]) {
                return v;
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

/// Read the `description` field from YAML frontmatter. Handles plain scalars
/// (with optional quotes) and block scalars (`|` literal, `>` folded). A
/// leading `|`/`>` may carry a `-`/`+` chomping indicator, which we ignore —
/// the result is always trimmed for display. Returns `None` only when the
/// field is absent (so the caller can fall back to prose).
fn front_description(front: &str) -> Option<String> {
    let lines: Vec<&str> = front.lines().collect();
    for (i, raw) in lines.iter().enumerate() {
        let line = raw.trim();
        let Some(v) = line.strip_prefix("description:") else {
            continue;
        };
        let v = v.trim();
        let first = v.chars().next();
        if first == Some('|') || first == Some('>') {
            return Some(collect_block_scalar(&lines, i, first == Some('>')));
        }
        // Empty value on this line: tolerate a value on the next indented line.
        if v.is_empty() {
            if let Some(val) = next_indented_value(&lines, i) {
                return Some(clean_value(&val));
            }
            return Some(String::new());
        }
        return Some(clean_value(v));
    }
    None
}

/// Collect a `|` / `>` block scalar starting after `lines[key_idx]`. Lines
/// indented strictly deeper than the key belong to the block; the first line
/// back at or above the key's indent ends it.
fn collect_block_scalar(lines: &[&str], key_idx: usize, folded: bool) -> String {
    let key_indent = leading_spaces(lines[key_idx]);
    let mut block: Vec<&str> = Vec::new();
    for line in lines.iter().copied().skip(key_idx + 1) {
        if line.trim().is_empty() {
            block.push(line);
            continue;
        }
        if leading_spaces(line) > key_indent {
            block.push(line);
        } else {
            break;
        }
    }
    // Strip the common leading indent from non-empty block lines.
    let common = block
        .iter()
        .filter(|l| !l.trim().is_empty())
        .map(|l| leading_spaces(l))
        .min()
        .unwrap_or(key_indent + 1);
    let stripped: Vec<String> = block
        .iter()
        .copied()
        .map(|l| {
            if l.trim().is_empty() {
                String::new()
            } else {
                l.get(common..).unwrap_or(l).to_string()
            }
        })
        .collect();
    let joined = if folded { fold_lines(&stripped) } else { stripped.join("\n") };
    joined.trim().to_string()
}

/// Fold block lines like YAML `>`: consecutive non-empty lines join with a
/// space; blank lines become a paragraph break.
fn fold_lines(lines: &[String]) -> String {
    let mut out = String::new();
    for l in lines {
        if l.is_empty() {
            if !out.is_empty() && !out.ends_with('\n') {
                out.push('\n');
            }
            continue;
        }
        if !out.is_empty() && !out.ends_with('\n') {
            out.push(' ');
        }
        out.push_str(l);
    }
    out
}

/// First non-empty line indented strictly deeper than `lines[key_idx]`, as a
/// trimmed string. Tolerates `key:` with the value placed on the next line.
fn next_indented_value(lines: &[&str], key_idx: usize) -> Option<String> {
    let key_indent = leading_spaces(lines[key_idx]);
    for line in lines.iter().copied().skip(key_idx + 1) {
        if line.trim().is_empty() {
            continue;
        }
        return if leading_spaces(line) > key_indent {
            Some(line.trim().to_string())
        } else {
            None
        };
    }
    None
}

fn leading_spaces(s: &str) -> usize {
    s.chars().take_while(|c| *c == ' ').count()
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_scalar() {
        let f = "\nname: foo\ndescription: 一个描述\n";
        assert_eq!(front_description(f).unwrap(), "一个描述");
    }

    #[test]
    fn quoted_scalar() {
        let f = "\ndescription: \"一个描述\"\n";
        assert_eq!(front_description(f).unwrap(), "一个描述");
    }

    #[test]
    fn literal_block_single_line() {
        // The case from the bug report: `description: |` with the value below.
        let f = "\nname: ima\ndescription: |\n  统一的 IMA OpenAPI 技能，支持笔记管理和知识库操作。\n";
        assert_eq!(
            front_description(f).unwrap(),
            "统一的 IMA OpenAPI 技能，支持笔记管理和知识库操作。"
        );
    }

    #[test]
    fn literal_block_multiline() {
        let f = "\ndescription: |\n  第一行\n  第二行\n";
        assert_eq!(front_description(f).unwrap(), "第一行\n第二行");
    }

    #[test]
    fn folded_block() {
        let f = "\ndescription: >\n  第一行\n  第二行\n";
        assert_eq!(front_description(f).unwrap(), "第一行 第二行");
    }

    #[test]
    fn block_with_chomping_indicator() {
        let f = "\ndescription: |-\n  第一行\n  第二行\n";
        assert_eq!(front_description(f).unwrap(), "第一行\n第二行");
    }

    #[test]
    fn empty_value_with_indented_continuation() {
        let f = "\ndescription:\n  统一描述\n";
        assert_eq!(front_description(f).unwrap(), "统一描述");
    }

    #[test]
    fn absent_field_returns_none() {
        let f = "\nname: foo\n";
        assert!(front_description(f).is_none());
    }
}
