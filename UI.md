# Aurora UI 开发规范

本项目 UI 用 MyGo（`github.com/egoist/mygo`）的 `ui` 包直接以 Go 绘制，无 HTML/前端。**任何 UI 改动前先读本文件**；改动涉及本文件描述的规范时，在同一改动里同步更新本文件。

## 设计原则

1. **对标 macOS 原生**：视觉水准参照 macOS 原生控件与 Lucide 图标；间距必须对齐，状态反馈缺一不可。
2. **状态完整性**：任何可交互元素都要考虑全部状态——默认 / hover / 选中 / 焦点 / 禁用，以及它们之间的切换不能引起布局抖动。
3. **单位全是 DIP**（设备无关像素）：`FontSize`/`Height`/`Padding` 等所有尺寸都是 DIP，框架按屏幕 scale（Retina=2）自动换算物理像素。不要手写 px 换算，直接用视觉舒服的数值（正文 12–13、标题 15、卡片圆角 10–12）。验证高分屏渲染可用 `tt.SetScale(2)`。
4. **用主题色，不写字面量**：`t.Accent`/`t.AccentText`/`t.Text`/`t.TextMuted`/`t.Surface`/`t.SurfaceHover`/`t.Border`/`t.Warning`/`t.Success`/`t.Danger`，浅色变体用 `t.Accent.Alpha(0.12)` 这类透明度派生。

## 页面骨架

- **窗口**：`TitleBarHiddenInset` + `VibrancySidebar`（红绿灯嵌入侧栏、毛玻璃），参考 MyGo 源码里的 `examples/vibrancy`（本项目的布局蓝本）。有 vibrancy 时根元素和侧栏背景设 `ui.Transparent`。
- **菜单栏**：`installMenu()`（main.go，`WhenReady` 里调用）把命令放进菜单栏——App 菜单的「设置…」（⌘, 跳设置页）、编辑菜单的「搜索…」（⌘F 聚焦当前页搜索框，经 `a.searchFocus` 传递）、视图菜单的「切换边栏」（⌃⌘S，与顶栏折叠按钮共用 `toggleSidebar()`）。菜单 Click 里用 `w.Update(func(){…})` 改状态并重绘。新增页面级命令时在这里补。
- **页面顶栏**：一律用 `paneHeader(c, 标题, actions)`（main.go），不要自己写。它的第一个子元素是侧栏折叠按钮（32×28 图标按钮，菜单 ⌃⌘S 共用 `toggleSidebar()`），之后是标题。左内边距 `headerLead()`：折叠时 `max(TitleBar().Left+8, 20)`（macOS 让开叠在内容区左缘的红绿灯），展开时 20；右内边距 `headerTail()` = `TitleBar().Right+20`——Windows/Linux 的最小化/最大化/关闭按钮叠在窗口右上角（macOS 的 `Right` 为 0，退化为 20）。actions 为页面的右上操作区（搜索框、主按钮），无操作传 nil。
- **侧栏品牌行**：侧栏顶部是 app 图标（`appIcon`，`resources/icon-128.png` 内嵌解码，`Radius(6).Clip()` 圆角）+ 名称「Aurora」（13pt Bold）。macOS 上红绿灯占侧栏顶部 52 高（`TitleBar().Left > 0` 时先垫一个 DragWindow 盒），品牌行在其下；Windows/Linux 窗口按钮在右上角，品牌行直接置顶（上 padding 加大到 14）。品牌行本身 `DragWindow()` 可拖窗口。
- **内容区**：卡片类页面（技能中心/技能包/Agent 技能）的滚动内容统一 `Padding(16, 28, 24, 28)` + `Gap(14)`，第一行通常是 12pt muted 的页面提示语；三个页面必须保持一致，改一处就改全部。设置页是表单式布局，用 `Padding(24, 28, 0, 28)` + `MaxWidth(640)`，是例外。
- **网格列数自适应**：卡片网格（技能中心、Agent 技能）的列数用 `gridColumns(c.Size(), a.collapsed)`（main.go）按窗口宽度取 1/2/3 列，不写死 `Columns(2)`（layout.md › Adaptability）。
- **弹窗**：`ui.Modal(c, &open, fn)` 自带遮罩、点外部/Escape 关闭。**Modal 面板自带 `Padding(t.Space(5))`（20）**，弹窗内容分 section（头部/内容/底部）且分隔线要通栏时，给内容根元素加 `.Margin(-t.Space(5))` 抵消，各 section 自带 padding（头 `14,16,12,16`、内容 `14,16`、底 `10,16`），section 之间 `ui.Divider`——所有弹窗（`import.go`、`settings.go`、`packs.go`）都是这个结构。右上角关闭 × 用 `modalClose(c, &open)`（settings.go）。

## 控件配方

### 侧栏（main.go）

- **彩色图标**：`ui.SidebarItem` 的图标走 `ui.Icon`，会把 SVG 强制染成文字色。要彩色方块图标：传透明占位 SVG（`spacerIcon`），再用 `ui.Image`（保留原色）绝对定位覆盖：`Left(t.Space(2)).Top(t.Space(1.5)).Size(16,16)`。
- **选中背景**：框架默认侧栏有焦点才蓝底、失焦灰底。本项目要求**无条件蓝底**：`item()` 里对选中项强制 `Background(t.Accent).TextColor(t.AccentText)`，尾注文字同步 `t.AccentText`（否则深色字压蓝底）。
- **折叠/展开丢焦点**：用 `FocusBind(&a.focusSide, "sidebar")`，展开时重新赋值 `a.focusSide = "sidebar"`；初始值 `"sidebar"` 让启动时侧栏自带焦点。
- **折叠按钮**：绝对定位 `Left(max(bar.Left+8, w-40))`，`w` 是动画中的侧栏宽度，折叠时跟随右边缘滑动，收起后停靠红绿灯旁。

### 输入框（hub.go `capsuleSearch`、import.go `importField`/`importTextArea`）

- 无样式输入：`TextInputBase`（单行）/`TextAreaBase`（多行）只给行为，**外观全靠自己画**：外层 `ui.Row`/`ui.Column` 画背景（`t.Surface`）、圆角、padding，内放输入 `.Grow(1)`。
- **焦点环**：`Children` 之后用 `if input.Focused()` 判断加 `Border(2, t.Accent.Alpha(0.45))`。**边框占据布局空间**：默认就要设同宽、与背景同色的边框（`Border(2, t.Surface)`），聚焦只改颜色，否则内容移位。
- 点击整壳聚焦：`field.OnClick(func() { input.Focus() })`，配 `.Cursor(ui.CursorText)`。
- **清除按钮**（`capsuleSearch` 专有）：`*value != ""` 时尾部条件渲染 16×16 的 ×（`clear:`+label 作 key），点击清空并 `input.Focus()` 保持焦点——搜索框的标准组成（search-fields.md）。为此胶囊的 padding 是 `(0, 6, 0, 12)`。
- **外部请求聚焦**：`capsuleSearch` 的 `focus bool` 参数在构建时 `input.Focus()`，给菜单的「搜索…」（⌘F）用；各页面把 `a.searchFocus` 取出并清零后传入，弹窗等无此需求的传 `false`。

### 列表项 / 卡片

- 自定义行用 `ui.ButtonBase(c.Key(k))`：注意它的 children **水平排列且主轴默认居中**——上下结构要再包 `ui.Column`，靠左加 `.Justify(ui.Start)`。
- 状态配色（解析结果行）：选中 `Border(1, t.Accent).Background(t.Accent.Alpha(0.08))`，hover `Background(t.SurfaceHover)`，默认 `Border(1, t.Border).Background(t.Surface)`。
- **分组列表**（settings.go `agentList`）：同类条目的列表用**一个圆角 10 容器**（`Radius(10).Background(t.Surface).Border(1, t.Border).Clip()`）装所有行，行间 1px hairline（`t.Border`）缩进与文字左缘对齐，不渲染每行独立描边卡片；列表的工程说明放容器下方 11pt muted 的 footer。行内结构：`agentRow` 是普通 `ui.Row`（不导航），hover 整行 `t.SurfaceHover`；行尾动作按钮用常驻低对比（默认 `t.TextMuted.Alpha(0.55)`、行 hover 加深、自身 hover 加底色），菜单双通道——按钮 `PopoverMenuButton` + 整行 `ContextMenu`（右键），内容相同。**容器内也可放表单行**（llm.go `llmRow`/`llmForm`：40 宽 muted 标签 + 控件 + 尾部动作，分隔线缩进 66），密钥类输入用 `ui.TextInput(...).Password()` 掩码、保存后只显示固定长度圆点。
- **Base 后缀的控件都是无样式的**：`CheckboxBase` 渲染为空；要复选框外观用 `ui.Checkbox(c, &checked, "")`（空 label 只画方框，a11y 名 `.Label()` 另给）。
- **嵌套可点击元素**：按下（press）目标是**最内层**控件——卡片里的按钮/输入框被点时，卡片的 OnClick 不触发。
- **OnClick 是延迟动作，Clicked() 是构建期查询**：`OnClick(fn)` 在构建 pass 结束后才执行，只能改应用状态，改构建期局部变量无效；需要本次构建内读结果（如记录待删索引）用 `if btn.Clicked()`。
- **遍历中改集合要延迟**：视图构建期间不要增删正被遍历的切片——记下索引，循环后再改（settings.go 的 `del` 模式）。

### 网格与文本

- `ui.Grid(c).Columns(n).Gap(g)` 子元素自动逐行填格并拉伸满格，同行等高；不要自己用 `Wrap()` 算换行。
- 文本截断 `MaxLines(n)`（自带省略号）；预留固定行高用 `FixedLineHeight(v)` + `Height(n*v)`（技能卡片简介固定两行，指标常量在 hub.go 顶部）。
- 字重：`Bold()` 或 `FontWeight(500)`（medium）；胶囊标签是 11pt medium。
- 非按钮元素也能用 `Hovered()` 做 hover 反馈（构建期查询）。

### 其他

- **分段控件**：`ui.Segmented` 的段不拉伸；macOS 样式用 `ui.SegmentedBase`：轨道 `Background(t.Surface)`，每段 `.Grow(1)`，选中段 `Background(t.Background).Shadow(0,1,2,0,ui.RGBA(0,0,0,0.12))`（settings.go `segment()`）。顶栏里的紧凑型不 `Grow`、段用 `Padding(4,10)` + 12pt 字，如技能中心的排序切换（hub.go `sortSwitch`）。
- **省略号惯例**：打开弹窗/新窗口收集输入的按钮标题带尾随省略号（buttons.md › Push buttons）——「导入技能…」「新建技能包…」「添加 Agent…」「＋ 添加技能…」；直接执行的（「解析」「创建」「取消」）不加。
- **徽章/标签**：`ui.Badge` 是给侧栏数字计数设计的（padding 极小），拿来做文字标签会很丑。文字标签一律用 `tag(c, text)`（settings.go）：`Padding(2,8)`、`Radius(5)`、`t.SurfaceHover` 底、10pt medium muted 字（如「已添加」）。蓝色「默认」徽章用 `defaultTag`（settings.go）：淡蓝底（`t.Accent.Alpha(0.12)`）+ 对勾图标 + 蓝字。带语义色的徽章（如「已存在」琥珀色）按同规格自制，见 `import.go` 的 `parsedRow`。
- **文字按钮**：列表行内的小型动作用 `linkAction(c, 标签, disabled, onClick)`（import.go）：12pt accent 蓝字、hover 淡蓝底、不适用时浅灰置灰（如解析列表的「全选」「取消选择」）。
- **胶囊标签**（hub.go `chip()`）：ButtonBase 开关，选中=蓝边/淡蓝底/黑字，未选中=灰边/白底/灰字，两态 hover 加深；各态边框同宽防抖动。**前缘图形区分状态**：选中 10px 对勾（`checkIcon`）、未选中 10px 空心圆（`circleIcon`）——形状+颜色双重区分（toggles.md），两态同尺寸，切换不改宽度，也不留空白占位。选中态**实时派生自文件系统**（`links.go` 的 `skillLink`），不存内存状态；点击动作用构建期 `Clicked()`（不是延迟的 `OnClick`），这样才能随即调 `c.Toast(...)` 提示冲突。技能包页复用同款两态：agent 胶囊（packs.go `packAgentChip`，整包链接开关）；技能成员胶囊**本体不可点**（普通 Row），仅尾部 20×20 的 × 按钮从包中移除（遍历中改集合用 `packRemoval` 延迟模式），中心已没有的技能用警告色「已丢失」样式（`packSkillChip`）。
- **选择器胶囊**（agentskills.go `agentPickChip`）：与开关胶囊区分的第三种胶囊——用于**单选切换**（Agent 技能页选 agent），选中=蓝边/蓝底/白字（实心填充），未选中=灰边/白底/黑字；同样各态边框同宽。**数量不限**：胶囊 `Wrap()` 自动换行，不折叠成 pop-up 按钮（用户决定，2026-11：宁可多占行高也要一眼看到全部 agent）。
- **语义色来源标签**（agentskills.go `agentSkillTag`）：tag 规格同 `tag()`（`Padding(2,8)`、`Radius(5)`、10pt medium），按语义配色：技能中心=淡蓝底蓝字，真实目录=淡绿底绿字（`t.Success.Alpha(0.12)`），外部软链接=灰底灰字；说明行里的关键词用同样的颜色（`agentSkillHint`）。
- **卡片一律无边框**：所有页面的卡片（技能中心、技能包、Agent 技能）都是 `Radius(12)` + `Background(t.Surface)`，不加 Border；来源/状态用标签表达，不用边框。技能中心卡片**本体是按钮**（ButtonBase 包一个 `Grow(1)` 的内容 Column，点击打开查看器，hover `t.SurfaceHover`）；内部的 agent 芯片和删除 × 是最内层控件，press 不冒泡到卡片。
- **轻提示**：操作结果的瞬时反馈用 `c.Toast("…")`（框架自带，窗口底部数秒），在 `Clicked()` 分支里调用；同名消息会自动刷新而不是堆叠。Tester 截图抓不到 toast（淡入动画首帧透明），用 `HasText` 断言其文字。
- **hover 才显示的控件**：构建期先查父元素 `Hovered()`，条件渲染 `Absolute()` 定位的按钮——`Absolute` 相对父元素 padding box 定位、不占布局，显隐不会引起抖动。例外是**常驻但低对比**的做法：技能卡片右上角的删除 ×（hub.go `skillCard`）总是渲染、默认 `t.TextMuted.Alpha(0.55)` 低对比、hover 才加底色加深——键盘用户与新用户也能发现它（accessibility.md 键盘-only 原则），用透明度代替显隐同样不引起抖动。
- **后台任务**：耗时操作（网络、克隆）放 goroutine，结果经包级变量 `runOnMain`（= `mygo.RunOnMain`）回主线程再改 app 状态；用代次计数（`parseGen`）防止旧任务结果覆盖新任务，可取消的任务存 `context.CancelFunc` 并在关闭入口 cancel。期间禁用相关按钮（`Disabled` + OnClick 里再判一次，Tester 点击无视 Disabled）。测试 stub `runOnMain` 为同步执行（`gitimport_test.go` 的 `stubParse`）。
- **确认对话框**：破坏性操作用框架自带 `ui.AlertDialog(c, &open, 标题, 消息, "取消", "删除")`，返回被点按钮的索引（未点 -1，Esc 是 `ui.AlertDismissed`=-2），点按钮后自动关闭；无需自制确认弹窗。见 hub.go `deleteSkillModal`。
- **行内 ⋯ 菜单**：用 `PopoverMenuButton`（settings.go `agentMore`），菜单随主题绘制在窗口内，打开在按钮下方。**菜单项的 `Chosen()` 在视图构建之外由框架单独重放**（`menuChoice`），处理函数只能直接改 app 状态（如 `deleteAgent`），不能依赖构建期局部变量——那是一次性闭包，写了也丢。
- **图标**：彩色方块图标加 `icons/*.svg` + embed 变量（`mygo dev` 不监听 SVG，需同时保存 .go 或重启 dev）；`currentColor` 染色的线条图标用 `lucide()` 助手（icons.go）包 24×24 Lucide 路径。
- **Row 交叉轴默认居中**：固定高度 Row 里的列要撑满用 `.AlignItems(ui.Stretch)`，否则内容整体居中、`Grow(1)` 分不到空间。元素顺序即文档顺序：沉底元素最后创建并带 `Grow(1)`。
- **要撑满的内容别放进 `ui.Scroll`**：Scroll 内容区高度由内容决定，内部的 `Grow(1)` 分不到空间。需要撑满可用高度的内容（如 粘贴/新建 的 SKILL.md 多行输入框）直接把表单列放进固定高度的父容器，见 import.go 弹窗对 method 4 的分支。

## 测试规范

- `ui.NewTester(view, w, h)`：`Click`(文本或 Label，**精确匹配**)、`HasText`(子串)、`ClickAt`(坐标)、`Find`(元素矩形，多匹配取第一个)、`Move`(悬停——hover 才渲染的控件要先 `Move` 到父元素再 `Click`)、`Image()`(像素采样做视觉回归)、`Advance()`(推进动画时钟，别用真实 sleep)、`Type`(键盘输入，注意点击控件会夺走输入框焦点)。
- 局限：假窗口永远聚焦，测不了"窗口失焦"；`c.Vibrancy()` 默认 false，用 `tt.SetVibrancy(true)` 切换。
- 视觉回归：采样关键像素断言（选中项蓝底、搜索框焦点环、元素位置不漂移），写法见 `main_test.go`、`hub_test.go`。
- 开发时的人工视觉检查：临时 `render_*_test.go` 渲染 PNG 到 /tmp，看完即删（详见 AGENTS.md 开发工作流）。
