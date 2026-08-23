package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xsift/sift/internal/config"
	"github.com/xsift/sift/internal/pi"
)

// sessionSpy records the interactive pi argv. Tests must swap this in: the
// production seam execs a real TUI.
type sessionSpy struct {
	args []string
	err  error
	n    int
}

func (s *sessionSpy) start(args []string) error {
	s.n++
	s.args = append([]string{}, args...)
	return s.err
}

func swapIssueSession(t *testing.T, fn func([]string) error) {
	t.Helper()
	old := startIssueSession
	startIssueSession = fn
	t.Cleanup(func() { startIssueSession = old })
}

func issueNewHome(t *testing.T) config.Home {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if os.Getenv("SIFT_HOME") == "" {
		_ = freshHome(t)
	}
	home, err := config.ResolveHome()
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func TestIssueNewLaunchesInteractiveWithMessage(t *testing.T) {
	issueTestProject(t)
	home := issueNewHome(t)
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)

	var out, errB strings.Builder
	code := runIssueNew([]string{"支持暗色主题"}, home, strings.NewReader(""), &out, &errB)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errB.String())
	}
	if spy.n != 1 {
		t.Fatalf("session starts=%d, want 1", spy.n)
	}
	joined := strings.Join(spy.args, " ")
	if containsToken(spy.args, "-p") || containsToken(spy.args, "--print") {
		t.Fatalf("args=%v must not use headless -p", spy.args)
	}
	if containsToken(spy.args, "--tools") || containsToken(spy.args, "-t") {
		t.Fatalf("args=%v must not narrow tools", spy.args)
	}
	if !containsToken(spy.args, "--append-system-prompt") {
		t.Fatalf("args=%v want --append-system-prompt", spy.args)
	}
	if spy.args[len(spy.args)-1] != "支持暗色主题" {
		t.Fatalf("args=%v want trailing initial message", spy.args)
	}
	prompt := appendPromptOf(spy.args)
	if !strings.Contains(prompt, "背景") || !strings.Contains(prompt, "触发标签") {
		t.Fatalf("append prompt lacks drafting rules: %s", prompt)
	}
	if strings.Contains(joined, "--tools") {
		t.Fatalf("unexpected tools in %q", joined)
	}
	if !strings.Contains(out.String(), "Sift skill 就绪") {
		t.Fatalf("stdout lacks skill line:\n%s", out.String())
	}
	skill := filepath.Join(os.Getenv("HOME"), ".pi", "agent", "skills", "sift", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("ops skill not written: %v", err)
	}
}

func TestIssueNewNoMessageOmitsInitialPrompt(t *testing.T) {
	issueTestProject(t)
	home := issueNewHome(t)
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)

	var out, errB strings.Builder
	code := runIssueNew(nil, home, strings.NewReader(""), &out, &errB)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errB.String())
	}
	if containsToken(spy.args, "-p") {
		t.Fatalf("args=%v", spy.args)
	}
	if spy.args[len(spy.args)-1] == "--append-system-prompt" {
		t.Fatalf("missing prompt value: %v", spy.args)
	}
	// After the flag+value pair there must be no leftover message token.
	if i := indexOf(spy.args, "--append-system-prompt"); i < 0 || i+2 != len(spy.args) {
		t.Fatalf("args=%v want only --append-system-prompt <text>", spy.args)
	}
}

func TestIssueNewUnknownFlagUsage(t *testing.T) {
	home := issueNewHome(t)
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)
	var errB strings.Builder
	code := runIssueNew([]string{"--wat"}, home, strings.NewReader(""), io.Discard, &errB)
	if code != 2 || spy.n != 0 {
		t.Fatalf("exit=%d starts=%d err=%s", code, spy.n, errB.String())
	}
	if !strings.Contains(errB.String(), "usage: sift issue new") {
		t.Fatalf("stderr=%s", errB.String())
	}
}

func TestIssueNewUnknownProjectDoesNotLaunch(t *testing.T) {
	issueTestProject(t)
	home := issueNewHome(t)
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)
	var errB strings.Builder
	code := runIssueNew([]string{"--project", "no-such"}, home, strings.NewReader(""), io.Discard, &errB)
	if code != 1 || spy.n != 0 {
		t.Fatalf("exit=%d starts=%d err=%s", code, spy.n, errB.String())
	}
	if !strings.Contains(errB.String(), "no-such") {
		t.Fatalf("stderr=%s", errB.String())
	}
}

func TestIssueNewAmbiguousProjectStillLaunches(t *testing.T) {
	home := issueNewHome(t)
	repo1 := filepath.Join(t.TempDir(), "one")
	repo2 := filepath.Join(t.TempDir(), "two")
	addTestProject(t, repo1, "git@github.com:owner/one.git")
	addTestProject(t, repo2, "git@github.com:owner/two.git")
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)

	var errB strings.Builder
	code := runIssueNew(nil, home, strings.NewReader(""), io.Discard, &errB)
	if code != 0 || spy.n != 1 {
		t.Fatalf("exit=%d starts=%d err=%s", code, spy.n, errB.String())
	}
	prompt := appendPromptOf(spy.args)
	if !strings.Contains(prompt, "one") || !strings.Contains(prompt, "two") {
		t.Fatalf("prompt should list both projects: %s", prompt)
	}

	code = runIssueNew([]string{"--project", "two", "补验收"}, home, strings.NewReader(""), io.Discard, &errB)
	if code != 0 || spy.args[len(spy.args)-1] != "补验收" {
		t.Fatalf("--project exit=%d args=%v err=%s", code, spy.args, errB.String())
	}
	if !strings.Contains(appendPromptOf(spy.args), "two") {
		t.Fatalf("flag project missing from prompt: %s", appendPromptOf(spy.args))
	}
}

func TestIssueNewPiMissingExitsWithGuidance(t *testing.T) {
	issueTestProject(t)
	home := issueNewHome(t)
	swapIssueSession(t, func([]string) error { return pi.PiMissingError{} })
	var errB strings.Builder
	code := runIssueNew([]string{"想法"}, home, strings.NewReader(""), io.Discard, &errB)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(errB.String(), "未检测到 pi") || !strings.Contains(errB.String(), "pi") {
		t.Fatalf("stderr lacks guidance:\n%s", errB.String())
	}
}

func TestIssueDispatchNewWithMessageIsLauncher(t *testing.T) {
	// Spec §3.1: first word `new` always enters the launcher, including an
	// unquoted remainder that used to be a Q&A question (#999 dispatch guard).
	_ = issueNewHome(t)
	spy := &sessionSpy{}
	swapIssueSession(t, spy.start)
	var out, errB strings.Builder
	code := runWithInput([]string{"sift", "issue", "new", "features", "planned"}, strings.NewReader(""), &out, &errB)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errB.String())
	}
	if spy.n != 1 {
		t.Fatalf("launcher starts=%d, want 1 (must not take Q&A)", spy.n)
	}
	if spy.args[len(spy.args)-1] != "features planned" {
		t.Fatalf("args=%v want joined message", spy.args)
	}
	if containsToken(spy.args, "-p") {
		t.Fatalf("dispatch leaked into headless -p: %v", spy.args)
	}
}

func containsToken(args []string, tok string) bool {
	for _, a := range args {
		if a == tok {
			return true
		}
	}
	return false
}

func indexOf(args []string, tok string) int {
	for i, a := range args {
		if a == tok {
			return i
		}
	}
	return -1
}

// testHome resolves the current SIFT_HOME without re-isolating. Callers that
// register a project first (issueTestProject) have already freshHome'd.
func testHome(t *testing.T) config.Home {
	t.Helper()
	if os.Getenv("SIFT_HOME") == "" {
		_ = freshHome(t)
	}
	home, err := config.ResolveHome()
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func appendPromptOf(args []string) string {
	for i, a := range args {
		if a == "--append-system-prompt" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
