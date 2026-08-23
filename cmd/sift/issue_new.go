// `sift issue new` is a shortcut into an interactive pi session with a
// drafting brief (ADR-015, specs/issue.md §3). The host stops at launch:
// discussion, create, and trigger labels happen inside pi. There is no
// second REPL and no register gate.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/xsift/sift/internal/config"
	"github.com/xsift/sift/internal/pi"
)

// startIssueSession launches the interactive pi TUI. Tests replace it so they
// never attach a real terminal.
var startIssueSession = func(args []string) error {
	return pi.RunSession(pi.OSRunner{}, args...)
}

const issueDraftAppend = "你正在帮操作者把一个想法写成高质量 forge issue。\n" +
	"- 补全结构：背景/问题/证据或复现/验收标准/范围边界。\n" +
	"- 需要澄清就直接问，一次问最关键的少数几条。\n" +
	"- 只基于已取证的事实；取不到的明说「取不到」。\n" +
	"- 创建 Issue 或打触发标签之前必须先问操作者并得到明确同意。\n" +
	"- 默认不建议打触发标签；操作者没要求就不要加。"

func runIssueNew(args []string, home config.Home, _ io.Reader, stdout, stderr io.Writer) int {
	projectID, message, err := parseIssueNewArgs(args)
	if err != nil {
		report(stderr, fmt.Errorf("usage: sift issue new [--project ID] [消息…]"))
		return 2
	}
	snap, err := config.Load(home, time.Now())
	if err != nil {
		report(stderr, fmt.Errorf("读取配置失败：%w", err))
		return 1
	}
	hint, err := draftProjectHint(snap, projectID)
	if err != nil {
		report(stderr, err)
		return 1
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		report(stderr, err)
		return 1
	}
	skillPath, err := pi.EnsureSkill(userHome)
	if err != nil {
		report(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ Sift skill 就绪：%s\n", skillPath)

	appendPrompt := issueDraftAppend
	if hint != "" {
		appendPrompt = issueDraftAppend + "\n\n" + hint
	}
	piArgs := []string{"--append-system-prompt", appendPrompt}
	if message != "" {
		piArgs = append(piArgs, message)
	}
	if err := startIssueSession(piArgs); err != nil {
		if _, ok := err.(pi.PiMissingError); ok {
			fmt.Fprintln(stderr, "✗ 未检测到 pi：`sift issue new` 依赖 pi。")
			fmt.Fprintln(stderr, piInstallManual())
			return 1
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintln(stderr, "✗ pi 会话结束：", err)
		return 1
	}
	return 0
}

var errIssueNewUsage = errors.New("usage")

func parseIssueNewArgs(args []string) (projectID, message string, err error) {
	var msg []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--project":
			if i+1 >= len(args) {
				return "", "", errIssueNewUsage
			}
			projectID = args[i+1]
			i++
		case strings.HasPrefix(a, "--project="):
			projectID = strings.TrimPrefix(a, "--project=")
		case strings.HasPrefix(a, "-"):
			return "", "", errIssueNewUsage
		default:
			msg = append(msg, a)
		}
	}
	return projectID, strings.Join(msg, " "), nil
}

func draftProjectHint(snap *config.Snapshot, projectID string) (string, error) {
	enabled := []config.Project{}
	if snap != nil && snap.Config != nil {
		for _, p := range snap.Config.Projects {
			if p.Enabled {
				enabled = append(enabled, p)
			}
		}
	}
	if projectID != "" {
		for _, p := range enabled {
			if p.ID == projectID {
				return formatDraftProject(p), nil
			}
		}
		return "", fmt.Errorf("未找到启用的项目 %q（sift project list 查看）", projectID)
	}
	if cwd, e := os.Getwd(); e == nil {
		match := ""
		uniq := true
		for _, p := range enabled {
			if p.Repo != "" && (cwd == p.Repo || strings.HasPrefix(cwd, p.Repo+string(os.PathSeparator))) {
				if match != "" {
					uniq = false
					break
				}
				match = p.ID
			}
		}
		if match != "" && uniq {
			for _, p := range enabled {
				if p.ID == match {
					return formatDraftProject(p), nil
				}
			}
		}
	}
	if len(enabled) == 0 {
		return "还没有绑定的项目。操作者可以之后用 sift project add 登记，或在会话里直接指定 forge 仓库。", nil
	}
	ids := make([]string, len(enabled))
	for i, p := range enabled {
		ids[i] = p.ID
	}
	return "已启用项目：" + strings.Join(ids, ", ") + "。不确定目标时先问操作者。", nil
}

func formatDraftProject(p config.Project) string {
	note := fmt.Sprintf("当前绑定项目：%s（%s %s）", p.ID, p.Forge.Kind, p.Forge.Project)
	if p.Repo != "" {
		note += "；仓库目录：" + p.Repo
	}
	if cwd, e := os.Getwd(); e == nil && p.Repo != "" && cwd != p.Repo && !strings.HasPrefix(cwd, p.Repo+string(os.PathSeparator)) {
		note += "。注意：当前目录不是绑定仓库，引用代码事实时必须注明来源。"
	}
	return note
}
