---
status: active
created: 2026-08-24
summary: Brain 可用 pi/claude/codex，init 默认 pi
---

# ADR-016：Brain 提供者为 pi / claude / codex

## 决策

`brain.executable` 可以是 pi、claude 或 codex（按 `agentfamily` 匹配 basename）。未写 `brain.protocol` 时按 family 推断外层信封；未写 `args` 时填该协议的默认 argv。`sift init` 在 Brain 为空且能解析到 pi 时默认写入 pi。不覆盖已有 `brain.executable`。Cursor 不是 Brain 提供者。

内层 T1–T7 closed schema 不变。外层新增 `pi-json-v1`、`codex-json-v1`；`claude-json-v1` 保持原样。禁止把 JSONL 猜成 Claude 信封。

## 理由

干净安装只登记编码 Agent、不写 Brain，T2 无法分派，Run 停在 queued。init 已引导装 pi，默认 Brain 用 pi 才能让首跑真正开工。三种 CLI 的 stdout 形状不同，必须分协议。

## 放弃的选项

| 选项 | 放弃理由 |
|---|---|
| 全部走 `claude-json-v1` | 违反「协议变了要新值、不靠 open-envelope 猜」 |
| 默认用已登记的第一个 Agent | 用户钉死默认 pi |
| 解析 Codex `config.toml` | 已明确暂缓 |

## 后果

- #1023 的「Brain 走 Codex」主路径由本决策覆盖（仍不读 toml）。
- 认不出 family 且未写 protocol 的 executable fail-closed。
- 已存在的空 queued Run 不会在补上 Brain 后自动重做 T2。
