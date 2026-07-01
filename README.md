# Aurora

A desktop app for managing skills across AI agents (Claude Code, Codex, Kimi Code, Opencode, and generic agents). Manage skills centrally, assign them to agents via symlinks/junctions, bundle skills into packs, and import external skills.

## Features

- **技能中心**：所有技能的中央仓库，每个技能含 SKILL.md 说明
- **技能包**：将常用技能打包为包，一键分配到多个 agent
- **Agent 技能**：按 agent 查看已分配的技能，可导入外部技能到中心统一管理
- **跨平台链接**：macOS/Linux 使用符号链接，Windows 使用 junction（无需管理员/开发者模式）
- **保护机制**：删除技能时只移除技能中心创建的链接，绝不触碰真实目录和外部链接

## Develop

```bash
# Install dependencies
pnpm install
```

### Dev Server

```bash
pnpm tauri dev
```

This launches a Vite dev server + the Rust backend. Changes to frontend code hot-reload; Rust changes trigger a recompile.

## Build

Builds are platform-specific — run the build command on each target platform.

### Prerequisites

- **Node.js 20+** + **pnpm 8+**
- **Rust 1.70+** (via `rustup`)

#### macOS
- Xcode command line tools: `xcode-select --install`

#### Windows
- [Visual Studio Build Tools](https://visualstudio.microsoft.com/downloads/) with the "Desktop development with C++" workload
- **No admin/Developer Mode required** — Windows builds use junctions instead of symlinks

#### Linux
```bash
# Debian/Ubuntu
sudo apt install libwebkit2gtk-4.1-dev build-essential curl wget file libssl-dev libayatana-appindicator3-dev librsvg2-dev
```

### Production Build

```bash
pnpm tauri build
```

Outputs per platform:
- **macOS**: `src-tauri/target/release/bundle/dmg/*.dmg` + `.app`
- **Windows**: `src-tauri/target/release/bundle/msi/*.msi`
- **Linux**: `src-tauri/target/release/bundle/deb/*.deb` + `.AppImage`

### Signing & Notarization (macOS)

Add to `tauri.conf.json` (fill in your own team ID / bundle ID):

```json
{
  "app": {
    "identifier": "com.your-domain.aurora",
    "windows": [{
      "title": "Aurora",
      "macOSPrivateKey": "Developer ID Application: Your Name (TEAMID)"
    }]
  }
}
```

Then build with:
```bash
APPLE_ID=your@apple.com APPLE_PASSWORD=app-specific-password APPLE_TEAM_ID=TEAMID pnpm tauri build
```

## Project Structure

```
├── src/                      # Frontend (React + TypeScript)
│   ├── components/
│   │   └── Sidebar.tsx      # 左侧导航
│   ├── pages/
│   │   ├── SkillsCenter.tsx # 技能中心
│   │   ├── Packs.tsx        # 技能包
│   │   ├── AgentSkills.tsx # Agent 技能
│   │   └── Settings.tsx     # 设置
│   └── lib/
│       ├── api.ts           # Tauri 命令调用
│       └── utils.ts
├── src-tauri/                # Rust 后端
│   ├── src/
│   │   ├── paths.rs         # 路径解析 + 跨平台链接检测
│   │   ├── meta.rs         # SKILL.md 解析
│   │   ├── packs.rs         # packs.json 读写
│   │   └── commands.rs      # Tauri 命令
│   ├── icons/               # 应用图标
│   └── tauri.conf.json
├── icon.png                  # 应用图标源文件
└── public/aurora-logo.png # 菜单栏 Logo
```

## Data Locations

- **技能中心**：`~/.aurora/skills/<skill-name>/`
- **技能包配置**：`~/.aurora/packs.json`
- **Agent 技能目录**：
  - Claude Code: `~/.claude/skills/`
  - Codex: `~/.codex/skills/`
  - Kimi Code: `~/.kimi-code/skills/`
  - Opencode: `~/.config/opencode/skills/`
  - 通用 Agent: `~/.agents/skills/`

## Cross-Platform Notes

| Aspect | macOS | Linux | Windows |
|--------|-------|-------|---------|
| Links | Unix symlinks | Unix symlinks | Junctions (no elevation needed) |
| Title Bar | Overlay + draggable top strip | Native | Native |
| Open dir | `open` | `xdg-open` | `explorer` |
| Icon format | `.icns` | set in `.desktop` | `.ico` |

