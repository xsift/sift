package brain

import (
	"errors"
	"testing"

	"github.com/xsift/sift/internal/storage"
)

func parseErr(t *testing.T, raw string) *EnvelopeError {
	t.Helper()
	_, _, _, err := ParseEnvelope([]byte(raw))
	var ee *EnvelopeError
	if !errors.As(err, &ee) {
		t.Fatalf("want EnvelopeError, got %v", err)
	}
	return ee
}

func TestParseEnvelopeValid(t *testing.T) {
	// Unknown diagnostic fields at the top level and inside usage are
	// accepted for forward compatibility (brain.md §4.1).
	raw := `{"type":"result","session_id":"s","cost":{"usd":0.1},
		"result_text":"{\"disposition\":\"ready\"}",
		"usage":{"input_tokens":11,"output_tokens":7,"cache_read":99}}`
	text, in, out, err := ParseEnvelope([]byte(raw))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if in != 11 || out != 7 {
		t.Fatalf("usage = %d/%d", in, out)
	}
	if string(text) != `{"disposition":"ready"}` {
		t.Fatalf("result_text = %q", text)
	}
}

func TestParseEnvelopeFailures(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		code string
	}{
		{name: "duplicate_key", raw: `{"result_text":"{}","result_text":"{}","usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
		{name: "trailing_text", raw: `{"result_text":"{}","usage":{"input_tokens":1,"output_tokens":1}} extra`, code: storage.ProviderErrInvalidEnvelope},
		{name: "top_array", raw: `[]`, code: storage.ProviderErrInvalidEnvelope},
		{name: "missing_result_text", raw: `{"usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
		{name: "missing_usage", raw: `{"result_text":"{}"}`, code: storage.ProviderErrUsageMissing},
		{name: "missing_usage_counter", raw: `{"result_text":"{}","usage":{"input_tokens":1}}`, code: storage.ProviderErrUsageMissing},
		{name: "usage_wrong_type", raw: `{"result_text":"{}","usage":{"input_tokens":"10","output_tokens":1}}`, code: storage.ProviderErrUsageInvalid},
		{name: "usage_negative", raw: `{"result_text":"{}","usage":{"input_tokens":-1,"output_tokens":1}}`, code: storage.ProviderErrUsageInvalid},
		{name: "usage_fractional", raw: `{"result_text":"{}","usage":{"input_tokens":1.5,"output_tokens":1}}`, code: storage.ProviderErrUsageInvalid},
		{name: "result_text_array", raw: `{"result_text":"[]","usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
		{name: "result_text_fenced", raw: `{"result_text":"` + "```json\n{}\n```" + `","usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
		{name: "result_text_trailing", raw: `{"result_text":"{} {}","usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
		{name: "result_text_scalar", raw: `{"result_text":"42","usage":{"input_tokens":1,"output_tokens":1}}`, code: storage.ProviderErrInvalidEnvelope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ee := parseErr(t, tc.raw)
			if ee.Code != tc.code {
				t.Fatalf("code = %q, want %q (%v)", ee.Code, tc.code, ee)
			}
		})
	}
}

func TestParseEnvelopeNonUTF8(t *testing.T) {
	raw := []byte(`{"result_text":"{}","usage":{"input_tokens":1,"output_tokens":1}}`)
	raw = append(raw[:len(raw)-1], 0xff, 0xfe, '}')
	ee := parseErr(t, string(raw))
	if ee.Code != storage.ProviderErrInvalidEnvelope {
		t.Fatalf("code = %q", ee.Code)
	}
}

func TestParseProtocolEnvelopeClaudeDelegates(t *testing.T) {
	raw := `{"result_text":"{\"disposition\":\"ready\"}","usage":{"input_tokens":3,"output_tokens":2}}`
	text, in, out, err := ParseProtocolEnvelope("claude-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in != 3 || out != 2 || string(text) != `{"disposition":"ready"}` {
		t.Fatalf("got %q %d/%d", text, in, out)
	}
}

func TestParseProtocolEnvelopePiJSONL(t *testing.T) {
	raw := "" +
		`{"type":"message_start","message":{"role":"assistant"}}` + "\n" +
		`{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"ignore"}]}}` + "\n" +
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{\"disposition\":\"ready\"}"}]},"usage":{"input":11,"output":7}}` + "\n"
	text, in, out, err := ParseProtocolEnvelope("pi-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in != 11 || out != 7 {
		t.Fatalf("usage = %d/%d", in, out)
	}
	if string(text) != `{"disposition":"ready"}` {
		t.Fatalf("result_text = %q", text)
	}
}

func TestParseProtocolEnvelopePiMessageUsageWins(t *testing.T) {
	// Real pi 0.84 --mode json: streaming message_update often reports 0/0;
	// the authoritative counters sit on message_end.message.usage.
	raw := "" +
		`{"type":"message_update","usage":{"input":0,"output":0}}` + "\n" +
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{\"ok\":true}"}],"usage":{"input":42,"output":7,"cacheRead":1024}}}` + "\n"
	text, in, out, err := ParseProtocolEnvelope("pi-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in != 42 || out != 7 || string(text) != `{"ok":true}` {
		t.Fatalf("got %q %d/%d", text, in, out)
	}
}

func TestParseProtocolEnvelopePiUsageTokenNames(t *testing.T) {
	raw := `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{}"}]},"usage":{"input_tokens":4,"output_tokens":5}}` + "\n"
	_, in, out, err := ParseProtocolEnvelope("pi-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in != 4 || out != 5 {
		t.Fatalf("usage = %d/%d", in, out)
	}
}

func TestParseProtocolEnvelopePiFailures(t *testing.T) {
	cases := []struct {
		name, raw, code string
	}{
		{name: "no_assistant", raw: `{"type":"message_end","message":{"role":"user","content":[{"type":"text","text":"{}"}]},"usage":{"input":1,"output":1}}` + "\n", code: storage.ProviderErrInvalidEnvelope},
		{name: "usage_missing", raw: `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{}"}]}}` + "\n", code: storage.ProviderErrUsageMissing},
		{name: "usage_negative", raw: `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{}"}]},"usage":{"input":-1,"output":1}}` + "\n", code: storage.ProviderErrUsageInvalid},
		{name: "not_json_object", raw: `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"[]"}]},"usage":{"input":1,"output":1}}` + "\n", code: storage.ProviderErrInvalidEnvelope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseProtocolEnvelope("pi-json-v1", []byte(tc.raw))
			var ee *EnvelopeError
			if !errors.As(err, &ee) || ee.Code != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}

func TestParseProtocolEnvelopeCodexJSONL(t *testing.T) {
	raw := "" +
		`{"type":"item.completed","item":{"type":"reasoning","text":"scratch"}}` + "\n" +
		`{"type":"item.completed","item":{"type":"agent_message","text":"{\"disposition\":\"ready\"}"}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":9,"output_tokens":6}}` + "\n"
	text, in, out, err := ParseProtocolEnvelope("codex-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if in != 9 || out != 6 || string(text) != `{"disposition":"ready"}` {
		t.Fatalf("got %q %d/%d", text, in, out)
	}
}

func TestParseProtocolEnvelopeCodexAgentMessageFallback(t *testing.T) {
	raw := "" +
		`{"type":"item.agent_message","text":"{}"}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n"
	text, in, out, err := ParseProtocolEnvelope("codex-json-v1", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "{}" || in != 1 || out != 1 {
		t.Fatalf("got %q %d/%d", text, in, out)
	}
}

func TestParseProtocolEnvelopeCodexFailures(t *testing.T) {
	cases := []struct {
		name, raw, code string
	}{
		{name: "no_agent_text", raw: `{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n", code: storage.ProviderErrInvalidEnvelope},
		{name: "usage_missing", raw: `{"type":"item.completed","item":{"type":"agent_message","text":"{}"}}` + "\n", code: storage.ProviderErrUsageMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseProtocolEnvelope("codex-json-v1", []byte(tc.raw))
			var ee *EnvelopeError
			if !errors.As(err, &ee) || ee.Code != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}

func TestParseProtocolEnvelopeUnknown(t *testing.T) {
	_, _, _, err := ParseProtocolEnvelope("mystery-v1", []byte(`{"result_text":"{}","usage":{"input_tokens":1,"output_tokens":1}}`))
	var ee *EnvelopeError
	if !errors.As(err, &ee) || ee.Code != storage.ProviderErrInvalidEnvelope {
		t.Fatalf("err = %v", err)
	}
}

func TestParseProtocolEnvelopeDoesNotGuessClaudeFromJSONL(t *testing.T) {
	raw := `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"{}"}]},"usage":{"input":1,"output":1}}` + "\n"
	_, _, _, err := ParseProtocolEnvelope("claude-json-v1", []byte(raw))
	if err == nil {
		t.Fatal("claude parser must not accept pi JSONL")
	}
}
