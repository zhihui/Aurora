# 项目
Aurora
## 项目类型 
基于 tauri 构建的桌面 app
## 功能
- 拥有一个 技能中心 功能，可以查看 skills 列表，删除 skill，通过弹出窗口查看 skill 的 SKILL.md，可以搜索技能名称，可以通过软链接将 Skills 中心的 skill 分配给 agent
  - 导入技能：支持从 GitHub 仓库/子目录、任意 URL（SKILL.md 或 zip/tar.gz）、本地文件夹/压缩包、粘贴 Markdown 文本 四种来源导入技能到技能中心（原「新建技能」已合并进导入）。下载/解压在 ~/.aurora/cache/import/ 暂存，解析后列出含 SKILL.md 的目录供勾选，可对同名冲突重命名后再复制进中心
  - 查看 SKILL.md 时支持「原文 / 中文」切换；中文为按需机器翻译（调用设置中配置的模型），译文缓存于 ~/.aurora/cache/translations/，按源文档内容哈希失效
- 拥有一个 技能包 功能，可以创建技能包，删除技能包，把 skill 添加进技能包，把技能包分配给 agent（将包中的所有 skill 通过软链接分配给 agent）
  - 「添加技能」通过弹窗选择：带搜索框（按名称或描述模糊匹配），列表项展示技能名 + 描述，可连续添加多个
  - 技能包内容变更会与已分配的 agent 自动同步：向已分配某 agent 的包中**添加**技能时，自动把该技能软链接进该 agent；**移除**技能时，删除该 agent 中对应的软链接（但若另一个仍分配给该 agent 的包也包含该技能，则保留；只删指向技能中心的软链接，不碰真实目录/外部链接）。这样技能包的「已分配」状态始终名副其实
- 拥有一个 Agent 技能 功能，可以给 agent 分配技能，移除技能
- 用有一个 设置 功能
  - 列出各大 agent 的 skill 目录
  - 配置大语言模型：端点、模型名称、密钥（密钥只可保存，不可查看；保存时留空表示保持原密钥不变）
- 所有搜索框（技能中心、技能包、Agent 技能）使用统一的模糊匹配（src/lib/utils.ts 的 fuzzyMatch）：按空格切分多个 token，各 token 需按顺序作为子串命中，如 "fro d" 命中 "front design"
- app 的配置和技能中心的内容存放在 ~/.aurora/
- 支持的 agents 有 
  - Claude Code : ~/.calude/skills/
  - Codex : ~/.codex/skills/
  - Kimi Code : ~/.kimi-code/skills/
  - Opencode : ~/.config/opencode/skills/
  - 通用 Agent: ~/.agents/skills/
- 拥有一个模型中心，可以添加模型的 provider, provider 下面可以添加多个模型 model。可以移除 provider
  - 模型 provider 拥有名称，英文标识，官网 URL （可选），API 端点URL， API 密钥字段。密钥字段只保存，不显示
  - 模型 model 拥有 模型id，模型显示名称字段，可以移除模型
## 注意
- 删除技能中心的 skill 时，应该将技能包中引用该 skill 的 skill 移除，将 agent 中的软链接删除
- agent 可能拥有与技能中心同名称的 real dir skill，或者非技能中心 skill 的软链接，删除技能中心的 skill 时不要删除 real dir skill 和非技能中心 skill 的软链接

## 技术栈
- 使用 tauri + rust + shadcn/ui
- 使用 pnpm
- shadcn/ui 使用主题使用 pnpm dlx shadcn@latest init --preset b115Qb7lLN --base base --template vite

## UI
- 支持响应式
- 左边为菜单，右边为内容区
- 右侧内容区左上、左下为圆角面板（rounded-l-xl），圆角缺口透出侧边栏底色
- 侧边栏品牌标题为 "Aurora"（无 logo 图标、不可选中）；菜单选中态用比侧边栏略深的同色调浅底（bg-foreground/[0.055]），文字用 text-foreground/65~80（不用纯黑）
- 隐藏标题栏，保留红绿灯按钮，设计一个透明的，与红绿灯高度一致的 app 顶部可拖拽条
- 注意 app 区域的内容不能与红绿灯重叠
- 当要设计一个全新的功能界面时，先设计一个 HTML 给用户审核，这样可以减少后期的修改成本
