# Aurora

用 Go + [MyGo](https://mygo.egoist.dev/docs)（`github.com/egoist/mygo`）编写的 macOS 风格桌面应用，管理 agent 技能（skill）：侧栏菜单为 技能中心 / 技能包 / Agent 技能 / 设置。采用 **native UI** 模式：界面由 MyGo 的 `ui` 包直接用 Go 绘制（GPU），没有 HTML/前端/webview。

## UI 开发（先读 UI.md）

**做任何 UI 改动前必读 [UI.md](UI.md)**（UI 开发规范：设计原则、页面骨架、控件配方、测试规范，含全部踩坑记录）。改动涉及 UI.md 描述的规范时，在同一改动里同步更新 UI.md。

## 配置目录（切记）

**本项目的配置目录是用户主目录下的 `.aurora/`**：macOS / Linux 为 `~/.aurora/`，Windows 为对应用户目录（`%USERPROFILE%\.aurora\`）。规则：

- **保存配置**：一律用 `config.go` 的 `configDir()`（`os.UserHomeDir()` + `filepath.Join(home, ".aurora")`）拼路径。**不要**用 `mygo.App.Path(mygo.PathUserData)`——那会落到平台各自的应用数据目录（如 macOS 的 `~/Library/Application Support/...`），违反本约定。
- **启动初始化**：应用启动时（`main.go` 的 `WhenReady`）调用 `ensureConfig()`，缺失则创建 `~/.aurora/`、`~/.aurora/skills/` 和空的 `agents.json`。新增必须的配置项时在这里补，并保证**已存在的配置绝不被覆盖**（先 `os.Stat` 判断）。
- **页面显示**：一律用 `config.go` 的 `displayHome(dir)` 缩短主目录前缀——macOS / Linux 显示 `~/…`，Windows 显示 `%USERPROFILE%\…`；不在主目录下的路径原样显示。
- **用户输入的路径**：做文件操作前一律用 `config.go` 的 `expandHome(dir)` 展开（`displayHome` 的逆操作）：Unix 的 `~/…`、Windows 的 `%USERPROFILE%\…` 和 `~\…` 都能展开，反斜杠会归一化为平台分隔符。配置里**存用户输入的原始形式**，展开只在使用时做。
- **跨平台铁律**：路径拼接只用 `filepath`（不是 `path`）；主目录只用 `os.UserHomeDir()`，绝不硬编码 `/Users/...`、`~` 或盘符；显示用 `displayHome`、展开用 `expandHome`。Windows 上同样能创建符号链接，但权限策略不同，文件操作失败要容忍（静默跳过），测试里 `os.Symlink` 失败用 `t.Skip`。
- 新增任何配置文件的保存或路径展示，都必须走这些函数；测试见 `config_test.go`。

## 初始化过程（已执行，勿重复）

```sh
go run github.com/egoist/mygo/cmd/mygo@latest init -template native -module aurora -name Aurora .
```

选择 `native` 模板的原因：只需要 Go（1.27+），不需要 Bun；web 模板才需要 Bun 跑前端工具链。CLI 以 Go tool 的形式钉在 `go.mod` 里，用 `go get -tool github.com/egoist/mygo/cmd/mygo@latest` 随 MyGo 一起升级。

## 常用命令

```sh
go tool mygo dev        # 开发：构建并启动 "Aurora Dev"，监听代码变化自动重建重启；编译失败时旧版本继续运行
CGO_ENABLED=0 go test ./...   # 测试：ui.NewTester 无窗口跑视图，模拟点击/输入/采样像素
go tool mygo vet .      # MyGo 专用静态检查（元素/句柄误存、goroutine 捕获等）
go tool mygo build      # 打包：输出 .app 和 .dmg 到 build/
gofmt -w .              # 提交前格式化
```

注意：

- 始终用 `CGO_ENABLED=0` 跑测试，MyGo 不用 cgo。
- `go build ./...` 会在根目录留下 `aurora` 二进制，验证完删掉；正式产物走 `mygo build`。
- 查框架 API 时读本地 pinned 源码：`go list -m -f '{{.Dir}}' github.com/egoist/mygo`，重点看 `ui/doc.go`、`ui/sidebar.go`、`ui/api_gen.go`、`ui/widgets.go` 和 `examples/vibrancy`。不要修改 module cache 里的文件。
- 项目有 `.agents/skills/mygo-maintenance` 技能，改 UI 前遵循其中的生命周期/焦点/线程规范。

## 开发工作流（用户要求，务必遵守）

1. **每次改完代码必须让用户看到运行中的应用**。平时用一个后台任务跑 `go tool mygo dev`（保存 .go 自动重建重启），改完确认日志最后一次是 `✓ Rebuilt`（中间的编译失败是分步编辑的过渡态，不用管）。会话重启后旧后台任务会失联：先 `pgrep -fl "Aurora Dev"` 杀掉残留进程，再重新后台启动 `go tool mygo dev`。
2. **视觉变更先截图自查**：写临时 `render_*_test.go`（`ui.NewTester` + `png.Encode` 输出到 /tmp），用 ReadMediaFile 检查两种状态（如聚焦/失焦、折叠/展开），确认后**删除临时测试和 png**。
3. **验证三件套**：`gofmt -w . && CGO_ENABLED=0 go test ./... && go tool mygo vet .`。
4. 改完检查是否产生多余文件（如 `go build` 留下的 `aurora` 二进制），删掉。

## 代码结构

- `main.go` — 应用状态（`app` 结构）、视图、侧栏、窗口配置、页面顶栏（`paneHeader`，所有页面顶栏必须用它）、**菜单栏**（`installMenu`：App 菜单「设置…」⌘,、编辑菜单「搜索…」⌘F 经 `a.searchFocus` 聚焦当前页搜索框、视图菜单「切换边栏」⌃⌘S 与顶栏折叠按钮共用 `toggleSidebar()`；菜单 Click 里 `w.Update` 改状态）、`gridColumns()`（卡片网格按窗口宽度取 1/2/3 列）
- `config.go` — 配置目录约定：`configDir()`（`~/.aurora/`，见上方「配置目录」）、`ensureConfig()`（启动初始化，缺失才建，绝不覆盖）、`displayHome()`（路径的分平台显示）、`expandHome()`（用户输入路径的展开，`displayHome` 的逆操作）
- `hub.go` — 技能中心页：顶栏右侧依次是胶囊搜索框（`capsuleSearch`，技能包页与 添加技能 弹窗共用；**有内容时尾部带清除按钮**，`focus` 参数给菜单 ⌘F 聚焦用）、排序分段控件（`sortSwitch`：名称/时间，`a.skillSort`，时间序新在前、无记录垫底）、「检查更新」按钮（`updateButton`，无 git 来源技能或检查/更新中禁用）+ 「导入技能…」按钮；内容区是技能卡片网格（`ui.Grid`，列数由 `gridColumns` 按窗口宽度取 1/2/3，简介固定两行高、agent 标签区自适应）。**卡片数据来自 `~/.aurora/skills/`**：`loadSkills()` 读取每个含 `SKILL.md` 的子目录（`parseSkill` 解析 front matter 的 name/description，目录名兜底；**只读文件头 16 KiB**——`readHead`/`skillHeadLen`，front matter 超长则回退目录名），按名称排序（视图再按 `skillSort` 重排；`skill` 结构还带 `.aurora.json` 的 `Meta`/`HasMeta`/`Updated`）。**卡片本体是按钮，点击打开查看器**（`openSkillView`，见 skillview.go；芯片与 × 是最内层控件，不会误触）。**技能的磁盘身份是目录名（`skill.Dir`），不是 front matter 的 name——两个目录可以声明同名**（如 `academy-guide/` 与 `academy-guide-2/`）：一切文件操作（链接、删除、技能包引用）都按 `Dir`，name 仅用于显示；Dir ≠ Name 时卡片标题旁显示灰色 `(目录名)` 以便区分。每张卡片为**所有 agent** 渲染胶囊标签 `chip()`，选中态实时派生自文件系统（`links.go` 的 `skillLink`）：agent 技能目录下存在指向本中心同名技能的软链接即选中；**选中态带对勾图标、未选中态带空心圆**（两态同尺寸防抖动，形状+颜色双重区分状态）。点击胶囊即 `toggleSkillLink`：已链接则删除软链接；未链接则创建（agent 目录缺失时一并创建）；该名称被**真实目录/文件/其他链接**占用时不做任何更改并 `c.Toast` 提示。卡片右上角有**常驻低对比**的 × 按钮（`Absolute` 定位，默认 muted 55% 透明、hover 加深，键盘用户也可达），点击弹 `ui.AlertDialog` 确认后调 `removeSkill` 删除技能（按 `a.delDir`），并从所有技能包中移除该技能（packs.go 的 `dropSkill`）。卡片底部有来源标签（`skill.Source`，读取技能目录内的 `.aurora.json`，无记录显示「未知来源」），其旁是 accent 色的「有更新」标记（`updateTag`，由 gitupdate.go 的检查结果 `a.updateAvail` 驱动），行尾右对齐显示「更新于 日期」（`skill.Updated` = 导入/最近更新时间，无记录不显示）。搜索过滤是纯前端逻辑（`matchSkill`，匹配名称/简介/agent 名）
- `links.go` — 技能与 agent 之间的软链接后端：`skillLink()`（none/ours/taken 三态，`samePath` 经 `EvalSymlinks` 归一后比较，macOS 的 /tmp 这类链接也能判等）、`toggleSkillLink()`（建/删链接，占用时返回给用户的消息）、`removeSkill()`（删除技能：先删所有 agent 中指向它的**本中心的**链接——别人的同名目录/链接不动——再删技能目录）、`packLinked()`/`togglePack()`（技能包整包建/删链接：全链接则全删，否则补齐未链接的；被占用的名字跳过并在消息中列名）。失效软链接视为占用（名字不空），由设置页的 `cleanBrokenLinks` 负责清理
- `packs.go` — 技能包页：把常用技能命名打包（`pack{Name, Skills[]}`，**Skills 存技能目录名即磁盘身份**，持久化到 `~/.aurora/packs.json`，懒创建，`packsPath` 为空时纯内存），一键把整包软链接给某个 agent。页面为单列卡片（无边框，surface 底）：包名 + 「N 个技能」tag + 重命名/删除图标按钮（`iconAction`，删除走 `ui.AlertDialog`，只删组合不动技能与链接）；技能成员胶囊本体不可点，仅尾部 20×20 的 × 按钮从包中移除（遍历中改集合用 `packRemoval` 延迟模式），中心已没有的技能显示警告色「已丢失」胶囊（× 同样移除）；「＋ 添加技能…」accent 胶囊打开挑选弹窗。底部「分配给：」是 agent 胶囊 `packAgentChip`（hub 胶囊同款两态，含选中对勾图标），选中 = 包内全部有效技能已链接（`packLinked`），点击即 `togglePack`；空包/全丢失点击只 toast 提示。**技能删除的两条路径都处理了**：应用内删除由 `dropSkill` 级联清包；应用外删除渲染成「已丢失」。弹窗三个：`packNameModal`（新建/重命名共用，重名红字 + 禁用）、`deletePackModal`、`pickSkillsModal`（搜索 + 技能列表，点击即加、可连续添加，已加的置灰带「已添加」tag）
- `agentskills.go` — Agent 技能页：查看某个 agent 已拥有的技能，分配或移除。内容区顶部依次是副标题（含「当前：X」）、通栏胶囊搜索、agent 选择控件（选择胶囊行 `agentPickChip`，选中=蓝底白字的**选择器**样式，与 hub 的开关胶囊不同；**数量不限**，胶囊自动换行）、来源标签说明行（`agentSkillHint`，关键词带语义色），下面是无边框卡片网格（列数 `gridColumns` 自适应）。**一切派生自文件系统**：`scanAgentSkills` 在视图构建时扫描 agent 技能目录，每个含 SKILL.md 的条目归类为——指向本中心的软链接（`agentSkillHub`，卡片带「技能中心」tag、路径前缀 `→` 显示解析后的目标、右下角「移除」按钮删链接）、外部软链接（`agentSkillLink`，灰 tag）、真实目录（`agentSkillReal`，绿 tag）——后两类带「导入」按钮；agent 目录就是本中心目录时算 `agentSkillOwn`，不给操作按钮。失效链接与无 SKILL.md 的条目跳过。**导入**（`importAgentSkill`）：复制内容进 `~/.aurora/skills/`（同名覆盖，复用 `importSkill`）→ 删除 agent 里的软链接或真实目录（`os.RemoveAll` 对软链接只删链接本身，外部目标不动）→ 在原位置创建指向本中心的软链接；成功后 `loadSkills()` 刷新中心列表。**前置防护**：条目已是指向本中心的链接（`linkOurs`，重复导入/双击场景）时直接拒绝，避免先删中心副本再读源的自毁
- `import.go` — 「导入技能」弹窗：左侧 5 种导入方式，右侧表单，底部取消/导入。**5 种方式全部已接入真实后端**：GitHub/命令行/链接 URL/本地 的解析共用 `startParse`（**后台 goroutine** 获取，结果经 `runOnMain` 回主线程应用，`parseGen` 代次防旧结果覆盖新解析，过期结果的临时目录会被清理），期间「解析中…」按钮与左侧导入方式均禁用；**取消/×/切换方式会 cancel 解析并清理临时文件**（`closeImport`）。**本地方式没有解析按钮**：选文件夹/选压缩包后立即解析（系统对话框经 `pickFolder`/`pickArchive` 变量在 goroutine 中调用，`mygo.Dialog.Open` 会阻塞调用方）。**粘贴/新建 方式没有解析**：底部按钮变为「创建」（名称或内容为空时禁用），`importPasted` 校验后写入（pasteimport.go）；其表单不放 `ui.Scroll`，直接撑满弹窗中段高度。解析结果共用 `applyParsed` 填入列表（`importSource`/`importOrigin`/`importCommit` 记录来源供 `importSelected` 写入 `.aurora.json`）。**导入已存在的技能 = 覆盖**（先整体删旧目录再复制），解析列表没有重命名输入框；列表顶部状态行右侧有「全选」「取消选择」（`selectAllParsed`，不适用时置灰，样式为文字按钮 `linkAction`）。解析/导入失败显示在表单下方的红色 `a.importErr`（`importErrRow`）
- `pasteimport.go` — 粘贴/新建 后端：`checkPastedSkill`（校验名称可作目录名、SKILL.md front matter 必须含 name 和 description，YAML 块标量也算）、`createPastedSkill`（覆盖式写入 + `.aurora.json` 记录 `source: "paste"`，卡片来源标签显示「手动创建」）
- `localimport.go` — 本地导入后端：`scanLocalFolder`（就地扫描用户选的文件夹，不复制不删除）、`unpackLocalArchive`（读 zip 文件后复用 `unzipSkills`，同限额）、`linkSkill`（**链接导入**：在 `~/.aurora/skills/` 下创建指向源技能目录的目录链接——`dirLink` 在 Windows 上先试 symlink、失败退回 `mklink /J` junction，见 `link_windows.go`/`link_other.go`；**链接导入不在用户目录里写 `.aurora.json`**，`loadSkills` 用 `Lstat` 认出软链接条目并把来源标签显示为「本地链接」；删除链接导入的技能只删链接，源目录不动）。`pickFolder`/`pickArchive` 是 `mygo.Dialog` 的系统选择框（变量，测试可 stub；返回 "" 即用户取消）
- `urlimport.go` — 链接 URL 导入后端（`net/http`，无外部依赖）：`fetchURL`（只接受 http/https，**下载上限 10 MiB**，可取消，出错不留临时目录）、`isZip`（magic bytes 或 .zip 后缀）、`singleSkill`（单个 SKILL.md 成一个技能，name 来自 front matter 或 URL 路径的父目录，网页/404/无名都报错，name 用 `validSkillName` 校验防目录逃逸）、`unzipSkills`（解压到临时目录后 `findGitSkills`，**防 zip-slip 路径逃逸**，解压上限 25 MiB / 1000 文件——限额参照 skills CLI）
- `cmdimport.go` — 命令行导入后端：`parseSkillsCmd`（`splitShell` 带引号/转义分词后，识别 `npx skills add` / `skills add` / `npx -y skills@latest add` 等前缀，提取来源与 `--skill` 过滤，`-g`/`-a`/`-y` 等安装选项忽略）、`localSkillDir`（`.`/`~`/`%`/绝对路径开头或实际存在的目录算本地来源，`expandHome` 展开）、`fetchCmd`（本地目录就地扫描——**不复制不删除用户目录**，`cmdResult.root` 为 ""；其余交给 `gitimport.go` 的 `gitFetch`）、`filterFound`（`--skill` 过滤，`*` 为全部，找不到报错；git 来源过滤失败时清理临时克隆）
- `gitimport.go` — Git 导入后端（纯 Go 的 **go-git**，不依赖系统 git 命令，Windows 也能用）：`parseGitURL`（owner/repo 简写、http(s)、scp 式 SSH、`/tree/` 与 GitLab 的 `/-/tree/` 子目录链接）、`resolveTreeRest`（按远程分支列表取**最长匹配**的分支前缀，其余为子目录）、`findGitSkills`（遍历找含 SKILL.md 的目录，技能目录不再深入，跳过隐藏目录）、`importSkill`（`os.CopyFS` 覆盖式复制 + 写入 `.aurora.json` 来源记录；agent 软链接指向路径不受覆盖影响；**源与目标相同则拒绝导入**——`samePath` 会解析软链接，防止「导入指向中心自身的链接」或「本地导入选了中心目录」时先删目标再读源导致数据丢失）、`sourceLabel`/`gitHost`（卡片来源标签：git→域名、local→本地文件、无记录→未知来源）。**导入来源记录在技能目录内的 `.aurora.json`**（`importMeta`：source/url/commit/importedAt + **branch/path**——更新 git 技能的依据：导入时跟踪的分支与技能在仓库内的路径，旧记录没有这两个字段，更新时重新解析分支、按名字回退匹配并自愈补齐），随技能删除一起消失，删除技能无需额外清理
- `gitupdate.go` — git 技能的**检查更新/更新**：`checkTargets`（按 `parseGitURL` 归一的 cloneURL 分组，**每个仓库只调一次** `listRemoteRefs` 即 go-git 的 ls-remote，与各技能记录的 commit 比较；结果按 cloneURL+分支分组成 `updateGroup`——同仓库不同子目录链接、不同导入时间的技能合并成一组），`skillBranch`（分支判定：meta.Branch → URL 的 /tree/ 尾巴 → 远程 HEAD → main/master 兜底）、`runUpdates`（**每组一次浅克隆** `updateClone`，按 `locateUpdate` 定位——记录的 path 优先，无 path 按目录名/声明名回退、歧义跳过——复用 `importSkill` 覆盖，记录刷新 commit 并自愈补齐 branch/path；agent 软链接指向路径不受影响；上游已消失的技能只报告不删本地；组失败不影响其他组）。后台 goroutine + `runOnMain` + `updateGen` 代次 + `updateCancel`，与导入解析同款模式。UI 是 `updateModal`（按仓库分组列出可勾选技能，行尾待更新 commit 短哈希 / 绿「已更新」/ 红失败原因；检查无更新时显示「已是最新」，检查/更新的失败红字列在列表上方；完成后底部变「完成」）
- `skillview.go` — 技能查看器弹窗（hub 卡片点击打开）：头部标题 + 路径副标题 + 原文/中文分段控件（`viewTabSwitch`），内容区 mono 文档盒（`viewDoc`）。**中文 tab** 三态：有新鲜译文（哈希匹配）显示译文 + 「由 X 翻译于 日期」；译文过期（SKILL.md 哈希变了）显示旧译文 + 警告行 + 「重新翻译」；无译文显示「翻译」按钮（未配置 LLM 时显示去设置的提示，不给按钮）。翻译走 `startTranslate`（后台 goroutine + `runOnMain` + `translateGen` 代次，与导入解析同款模式），上限 200 KiB
- `translate.go` — 技能翻译后端：`translation`（译文记录 `.aurora-zh.json`：sourceHash/model/translatedAt/content，存技能目录内，随技能删除消失）、`skillHash`（SKILL.md 的 SHA-256，判定译文是否过期）、`llmChat`（变量，测试可 stub）按配置类型走两种协议——`chatOpenAI`（POST {endpoint}/chat/completions，Bearer）与 `chatAnthropic`（POST {endpoint}/messages，x-api-key + anthropic-version），`apiURL` 智能拼路径（endpoint 已带 /v1 不重复）；`translateSystem` 是翻译指令（保 Markdown 结构、name 值/代码块/链接不译）
- `open.go` — `openDir`（包级变量，测试可 stub）：在系统文件管理器中打开目录，macOS 用 `open`、Windows 用 `explorer`、Linux 用 `xdg-open`；目录不存在直接报错（打开绝不创建目录）；`cmd.Start` 后不查退出码（explorer 成功也返回非零），`go cmd.Wait()` 只回收子进程
- `settings.go` — 设置页：agent 管理，`ui.Modal` 弹窗，预设/自定义两个标签页，agents 持久化到 `~/.aurora/agents.json`（`agentsPath` 为空时纯内存，测试用）。**agent 列表是 macOS 分组列表**（`agentList`）：一个圆角 10 容器（Surface 底 + 1px Border + Clip）装所有行，行间 hairline 缩进 62px 对齐名称左缘，容器下方 footer 显示配置目录与删除语义。**行不是按钮、不导航**（Agent 技能页顶部胶囊负责切换 agent）：`agentRow` = 36 avatar + 名称（内置 agent 名称旁带蓝色「默认」徽章 `defaultTag`）+ monospace 目录行 + 右侧「N 个技能」纯文字（`scanAgentSkills` 实时统计）+ ⋯ 按钮（24×24 无描边、常驻低对比、行 hover 加深）。菜单双通道内容相同（`agentMenu`）：⋯ 的 `PopoverMenuButton` 与整行 `ContextMenu`（右键）。**内置默认 agent「通用 Agent」**（目录 `~/.agents/skills`，`defaultAgent()` 按平台生成显示形式）：启动时 `ensureDefaultAgent()` 补入列表首位并纠正漂移的目录，**不可删除**（行尾 ⋯ 菜单只有「打开」，`deleteAgent` 也有拦截）。每行的 ⋯ 按钮（`agentMore`）弹 `PopoverMenuButton` 菜单：**打开** 用 `openDir`（open.go）在系统文件管理器中显示该 agent 的技能目录，**删除** 只删配置，绝不动目录。**注意菜单项的 `Chosen()` 在视图构建之外单独重放**（框架 `menuChoice`），处理函数只能直接改 app 状态，不能依赖构建期局部变量。页面为单滚动列两区块：Agents 区块（无 agent 时显示空状态盒）+ 大语言模型 API 区块（`llmSection`，见 llm.go）
- `llm.go` — 大语言模型 API 配置（`llmConfig`：type/endpoint/model/apiKey，持久化到 `~/.aurora/llm.json`，**权限 0600**，懒创建，`llmPath` 为空时纯内存）。类型两种（`llmTypes`：OpenAI 兼容 / Anthropic 兼容，即两种 API 协议；预填官方接口地址，**官方或第三方兼容地址均可**），表单里切换类型时接口地址**只在该字段为空或仍是另一预设默认值时**才换成新预设（用户自己填的不覆盖）。**密钥永不回显**：输入框用 `Password()` 掩码，保存后摘要行只显示固定 8 圆点（`maskedKey`，不泄露长度）；「更改」重开表单时密钥字段留空，**留空保存 = 保留原密钥**。保存校验：接口与模型必填，密钥在已有保存值时可为空。技能翻译功能从 `a.llm` 读取配置使用（见 translate.go）**添加 agent** 时会用 `expandHome` 展开其目录并调用 `cleanBrokenLinks` 清理其中的失效软链接（有效链接和普通文件不动；目录不存在不创建）
- `icons.go` + `icons/` — 侧栏彩色图标（`icons/*.svg` 嵌入 Lucide 图形，ISC 许可）+ `lucide()` 线条图标助手；`mygo dev` 不监听 SVG，单独改 SVG 需同时保存 .go 或重启 dev
- `main_test.go`、`settings_test.go`、`llm_test.go`、`hub_test.go`、`skillview_test.go`、`packs_test.go`、`agentskills_test.go`、`import_test.go`、`gitimport_test.go`、`gitupdate_test.go`、`cmdimport_test.go`、`urlimport_test.go`、`localimport_test.go`、`pasteimport_test.go` — 视图测试（写法见 UI.md 测试规范，git/URL/本地 导入用 stub 替换 `gitFetch`/`urlFetch`/`pickFolder`/`pickArchive` 变量免网络免对话框，git 更新用 stub 替换 `listRemoteRefs`/`updateClone`，翻译用 stub 替换 `llmChat`，URL 后端逻辑用 `httptest.Server` 本地起服务测真实下载，命令行导入的流程测试用真实本地目录 + stub `runOnMain`）；`links_test.go`、`config_test.go`、`translate_test.go` — 纯逻辑测试（软链接、路径与配置初始化、翻译协议与译文文件）
