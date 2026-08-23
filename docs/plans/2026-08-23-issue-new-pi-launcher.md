---
status: done
created: 2026-08-23
summary: 把 sift issue new 改成进 pi 的快捷引导
---

# `sift issue new` 快捷引导 Implementation Plan

> 规格：[ADR-015](../decisions/015-issue-new-is-pi-launcher.md)、[`specs/issue.md`](../specs/issue.md) §3。用户要求写完即实施，本会话内联执行；不按任务提交（未要求 commit）。

**Goal:** `sift issue new [消息…]` 只启动交互式 `pi`（可带首句），删掉宿主 REPL 与登记闸门。

**Architecture:** 分发上首词 `new` 一律进启动器。启动器幂等写入操作 skill、用 `--append-system-prompt` 追加起草说明、`exec pi`（有消息则作为位置参数）。测缝 `startIssueSession` 记录 argv，禁止跑真 TUI。

**Tech Stack:** Go；复用 `internal/pi.EnsureSkill` / `RunSession`。

---

### 文件

- Modify: `internal/pi/pi.go` — `Runner.RunInteractive`；`RunSession` 转发 extra args
- Modify: `internal/pi/pi_test.go` — 断言 argv 转发
- Rewrite: `cmd/sift/issue_new.go` — 启动器；删除 REPL/围栏/CreateIssue
- Rewrite: `cmd/sift/issue_new_test.go` — 对照 spec §5
- Modify: `cmd/sift/issue.go` — `new` 一律分发
- Modify: `cmd/sift/commands.go`、`cmd/sift/main.go` — usage / 注释
- Modify: `docs/specs/issue.md` — `draft` → `active`

### Task 1: 测试先红

- [x] `issue_new_test.go`：消息走启动器（无 `-p`/`--tools`，有 `--append-system-prompt` + 首句）；无消息不传首句；未知旗标退出 2；未知 `--project` 不启动；多项目仍启动；pi 缺失指引 + 退出 1；`sift issue new features planned` 不再走问答
- [x] `pi_test.go`：`RunSession` 把 extra args 交给 `RunInteractive`

### Task 2: 实现变绿

- [x] 扩展 `RunSession`
- [x] 重写 `runIssueNew` + 分发
- [x] 删死代码；usage；规格改 `active`

### Task 3: 验证

- [x] `go test ./internal/pi ./cmd/sift` 相关测试全绿
