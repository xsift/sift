---
status: active
created: 2026-08-23
summary: sift issue 只读问答与 issue new 进 pi 的契约
---

# Issue CLI 规格

本文是 `sift issue` / `sift issue new` 的命令契约。只读问答与 `issue new` 快捷引导均以本文件为准；`issue new` 的决策见 [ADR-015](../decisions/015-issue-new-is-pi-launcher.md)。`active` 不表示阶段门禁已过。

用户入口：[Getting Started §7.6](../guides/getting-started.md#76-单发语义问答sift-issue) / [§7.7](../guides/getting-started.md#77-起草快捷入口sift-issue-new)。Forge 读写能力仍以 [`forge.md`](forge.md) 为准；本文不新增 daemon 协议。

## 1. 入口分工

| 命令 | 形态 | 宿主做什么 |
|---|---|---|
| `sift pi` | 交互 TUI | 写入操作 skill，`exec pi` |
| `sift issue` / `list` / 自然语言 | 只读、非会话 | 列 issue 或一轮 headless pi |
| `sift issue new [消息…]` | 交互 TUI | 写入操作 skill、追加起草说明、`exec pi`（可带首句） |

三条命令不共享第二套对话循环。`issue new` 不是 REPL，也不是写 forge 闸门。

## 2. `sift issue`（只读，已合入）

首词精确匹配才走子命令，避免肌肉记忆烧掉一次模型调用。

| argv | 路径 |
|---|---|
| 无参数或 `list`/`ls`（后接仅 `--` 旗标） | 确定性列出启用项目的 open issue，零模型调用 |
| `--all` 出现在子命令前 | 列表/问答扩到全部启用项目；默认按 cwd 落在某个已绑定 `repo` 上则只查该项目 |
| 首词为 `close`/`reopen`/`edit`/`comment`/`label`/`assign` | 拒绝并提示改用 `gh`/`glab`，不调用 pi |
| 其余（含未加引号的问句） | 宿主只读取证后一轮 headless `pi --tools read` |

只读保证在代码里：forge 口无写方法；headless 工具白名单钉死 `read`。pi 未安装时降级为确定性列表 + 与 init 相同的安装指引，退出 0。写动词预留是为了以后真做时不破坏分发，不是本规格的实现范围。

`new` 不再走本条问答路径，见 §3。

## 3. `sift issue new`（快捷引导，目标契约）

### 3.1 分发

首词 `new` **一律**进本命令（可后接旗标与消息）。不再把 `sift issue new 未加引号的问句` 当成 §2 问答——那是为 `sift issue new "想法"` 让路的刻意破坏。

```text
sift issue new [--project ID] [消息…]
```

- `--project` / `--project=` 指定绑定项目 id，写入追加说明，不改变启动后的会话协议。
- 未知旗标：usage，退出 2。
- 消息为其余 argv 以单空格拼接；无消息则不传初始 prompt，进入空的交互会话。

项目提示（只进 prompt，不决定能否启动）：`--project` 命中启用项目则用它；否则 cwd 落在唯一已绑定 `repo` 内则用它；再否则列出已启用项目 id，仍启动会话。`--project` 给了但找不到启用项目：报错退出 1，不启动 pi。

### 3.2 启动

与 `sift pi` 相同的 TTY 纪律：stdin/stdout/stderr 原样交给子进程，不得把取证包或说明经 stdin 灌入（stdin 灌入会卡住 TUI）。

启动步骤：

1. 与 `sift pi` 一样幂等写入操作 skill（`~/.pi/agent/skills/sift/SKILL.md`）。
2. 本调用用 `--append-system-prompt` 追加起草说明（仅本次 argv，不写磁盘）。不得把起草说明装成 `~/.pi/agent/skills/` 下会被普通 `sift pi` 自动发现的 skill。
3. `exec` 交互式 `pi`：有消息则为 `pi <消息>`（pi 的「带首句进 TUI」形态），无消息则为 `pi`。禁止 `pi -p`：那会跑完一轮就退出，不是会话。
4. 不传 `--tools` 收窄。创建 issue、打标签、读代码都在 pi 里用它的默认工具完成。
5. 不预塞 open issue 取证包；需要对照时由会话自己取证。
6. 不建 `$SIFT_HOME/issue-sessions`，不解析 ` ```issue ` 围栏，不提供「登记 / 草稿 / #N」宿主命令。

起草说明只要求：补全背景/问题/证据或复现/验收/范围；创建或打触发标签前先问操作者；默认不建议打触发标签；取不到的事实明说。说明不是闸门。

### 3.3 失败

pi 不在 PATH：打印与 `sift pi` / init 相同的安装指引，退出 1。`pi` 进程的退出码原样作为本命令退出码（除上述 usage / 配置 / 未知项目）。配置读失败退出 1。

## 4. 明确不做

- 宿主确认、`$EDITOR` 改稿、标题查重、`CreateIssue`、触发标签二次确认。
- 为起草单独做一条只读工具白名单或第二套会话目录。
- 在 daemon、Gate、policy 里增加「谁可以建 Issue」。
- 改变 §2 只读问答的工具钉死或写动词拒绝。

## 5. 测试要点（实现时）

- `new` + 非旗标消息走启动器，不走 §2 headless `-p`。
- 启动 argv 含交互首句、不含 `-p`，且无 `--tools` 收窄。
- `--project` 未知 id 不 exec pi。
- 起草说明只出现在本次 `--append-system-prompt`，不写 `~/.pi/agent/skills/`。
- pi 缺失：指引 + 退出 1，不进入伪 REPL。
