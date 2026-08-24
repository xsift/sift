package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestBrainProtocolInferredFromExecutable(t *testing.T) {
	cases := []struct {
		exe   string
		proto BrainProtocol
		args  []string
	}{
		{"/usr/local/bin/pi", BrainProtocolPiJSONv1, []string{"-p", "--mode", "json", "--no-tools"}},
		{"claude", BrainProtocolClaudeJSONv1, []string{"-p", "--output-format", "json"}},
		{"/opt/codex", BrainProtocolCodexJSONv1, []string{"exec", "--json", "-"}},
	}
	for _, tc := range cases {
		t.Run(tc.exe, func(t *testing.T) {
			snap, err := mustLoadYAMLOrErr(t, "version: 1\nbrain:\n  executable: "+tc.exe+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if snap.Config.Brain.Executable != tc.exe {
				t.Fatalf("executable = %q", snap.Config.Brain.Executable)
			}
			if snap.Config.Brain.Protocol != tc.proto {
				t.Fatalf("protocol = %q, want %q", snap.Config.Brain.Protocol, tc.proto)
			}
			if !reflect.DeepEqual(snap.Config.Brain.Args, tc.args) {
				t.Fatalf("args = %#v, want %#v", snap.Config.Brain.Args, tc.args)
			}
		})
	}
}

func TestBrainProtocolExplicitWins(t *testing.T) {
	snap, err := mustLoadYAMLOrErr(t, "version: 1\nbrain:\n  executable: /opt/my-wrapper\n  protocol: pi-json-v1\n  args: [\"--x\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Config.Brain.Protocol != BrainProtocolPiJSONv1 {
		t.Fatalf("protocol = %q", snap.Config.Brain.Protocol)
	}
	if !reflect.DeepEqual(snap.Config.Brain.Args, []string{"--x"}) {
		t.Fatalf("args = %#v", snap.Config.Brain.Args)
	}
}

func TestBrainUnknownExecutableRequiresProtocol(t *testing.T) {
	_, err := mustLoadYAMLOrErr(t, "version: 1\nbrain:\n  executable: /opt/my-wrapper\n")
	if err == nil || !strings.Contains(err.Error(), "brain.protocol") {
		t.Fatalf("want protocol required, got %v", err)
	}
}

func TestBrainEmptyExecutableStaysDeterministic(t *testing.T) {
	snap, err := mustLoadYAMLOrErr(t, "version: 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Config.Brain.Executable != "" {
		t.Fatalf("executable = %q", snap.Config.Brain.Executable)
	}
	if snap.Config.Brain.Protocol != BrainProtocolClaudeJSONv1 {
		t.Fatalf("empty brain protocol = %q", snap.Config.Brain.Protocol)
	}
}

func TestBrainExplicitArgsPreserved(t *testing.T) {
	snap, err := mustLoadYAMLOrErr(t, "version: 1\nbrain:\n  executable: pi\n  args: [\"-p\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Config.Brain.Protocol != BrainProtocolPiJSONv1 {
		t.Fatalf("protocol = %q", snap.Config.Brain.Protocol)
	}
	if !reflect.DeepEqual(snap.Config.Brain.Args, []string{"-p"}) {
		t.Fatalf("args = %#v", snap.Config.Brain.Args)
	}
}
