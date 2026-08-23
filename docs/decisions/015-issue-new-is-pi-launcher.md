---
status: active
created: 2026-08-23
summary: issue new 只引导进 pi，宿主不再登记
---

# ADR-015：`sift issue new` 是进 pi 的快捷引导

## 决策

`sift issue new` 不再自造对话，也不再当写 forge 的宿主闸门。它只做一件事：带上起草说明（可选首句）启动与 `sift pi` 相同的交互 TUI。讨论、改稿、创建 Issue、是否打触发标签都在 pi 里完成。

契约见 [`specs/issue.md`](../specs/issue.md) §3。

## 理由

#999 的宿主 REPL（每句同步 `pi -p`、登记前宿主确认）有两个产品问题：发出去要等一整轮 headless；对话心智与 `sift pi` 重叠。pi 已支持 `pi "首句"` 进 TUI 并先答一轮。登记闸门若仍由宿主拦截，进了 TUI 就看不到「登记」二字，只能再做退出后确认或第二条命令——比不过「都在 pi 里做完」。

skill / 起草说明不是安全层。这与 `sift pi` 已接受的边界相同：最坏情况等价于一个手快的人类用户。Sift 真正的闸门仍在 approve nonce、policy fail-closed、`auto_merge` 默认关，不在建 Issue。

## 放弃的选项

| 选项 | 放弃理由 |
|---|---|
| 保留宿主 REPL，只改成真 pi TUI | 治等待、不治与 `sift pi` 重叠 |
| 退出 pi 后再由宿主确认登记 | 宿主仍管写路径，和「只是快捷引导」冲突 |
| `issue new "想法"` 只跑一轮 headless 就确认 | 没法接着改，又把长等待留在宿主 |

## 后果

- 实现须删掉 #999 REPL、围栏解析、`$SIFT_HOME/issue-sessions` 与宿主 `CreateIssue`。
- `sift issue new <消息>` 的分发从「未加引号则当问答」改为「首词 `new` 一律进启动器」。
- 指挥层文档不再把 `issue new` 写成写路径边界。
