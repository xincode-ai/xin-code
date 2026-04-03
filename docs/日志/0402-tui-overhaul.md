# 0402 TUI 交互界面重构 — 对标 Claude Code 视觉风格

## 背景
用户反馈 Xin Code 界面"太丑了"，与 Claude Code 和 CodeAny 差距明显。主要问题：thinking 多行刷屏、工具输出无格式、消息间无层次感、无 Markdown 渲染。

## 改动

### 消息渲染重构（chat.go）
- 问题：thinking 每条 MsgThinking 都新建 ChatMessage，导致多行 `💭` 刷屏
- 方案：合并连续 thinking 为一条消息，渲染为单行 `∴ thinking`
- 耗时：~15min

### 工具显示对标 CC（chat.go）
- 问题：工具显示为 `✓ 工具名` + 原始输出，无参数预览、无缩进、无折叠
- 方案：
  - 执行中：`⏺ Bash(command preview)`
  - 完成：`✓ Bash(cmd)` + `⎿` 缩进输出
  - 自动折叠 >8 行输出，显示 3 行预览 + `… +N lines`
  - `toolArgPreview()` 从 JSON 输入提取关键参数（command/path/pattern/url/query）
  - 工具名 bold（StyleToolName），参数 dim（StyleHint）
- 耗时：~30min

### ToolID 匹配 bug 修复（chat.go）
- 问题：MsgToolDone 按 `ToolName` 倒序匹配，连续两个 Bash 调用时第二个 Done 会覆盖第一个
- 方案：ChatMessage 加 ToolID 字段，MsgToolDone 按 ID 精确匹配
- 耗时：~5min

### AI 响应前缀（chat.go）
- 问题：assistant 消息无视觉标记，与其他消息混在一起
- 方案：完成态加 `●` 前缀（ColorText），流式态加 `●`（StyleToolRunning 蓝色）+ `▊` 光标
- 耗时：~10min

### 布局溢出修复（app.go — Codex review 发现）
- 问题：新增分隔线和 InputFrame 边框后，chat 区域高度计算仍用旧值，总行数超过终端高度
- 方案：statusH=3, sepH=2, inputH=4, chatH=height-9（含 min 5 保护）
- 耗时：~5min

### Markdown 换行宽度（chat.go — Codex review 发现）
- 问题：`● ` 前缀占 2 列，但 Glamour 仍按 `width-4` 换行，第一行可能溢出
- 方案：改为 `width-6`
- 耗时：~2min

## 提交
- `17286bf` feat: TUI 全面对标 Claude Code 视觉风格
- `798ea63` fix: 修复布局溢出 + Markdown 换行宽度

## 遗留
- 无工具执行 blinking 动画（CC 有）
- 无 thinking 展开/折叠交互（CC 有 Ctrl+O）
- 流式期间无 Markdown 渲染（完成后才渲染）
- `containsANSI` 可能误判含 `rgb:` 的合法输入
