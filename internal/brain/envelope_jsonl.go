package brain

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/xsift/sift/internal/schema"
	"github.com/xsift/sift/internal/storage"
)

func parsePiJSONL(raw []byte) ([]byte, int64, int64, error) {
	var text string
	var haveText bool
	var inTok, outTok int64
	var haveUsage bool
	err := eachJSONL(raw, func(obj map[string]any) error {
		msg, _ := obj["message"].(map[string]any)
		for _, usage := range []any{obj["usage"], msg["usage"]} {
			if usage == nil {
				continue
			}
			in, out, uerr := readUsage(usage, true)
			if uerr != nil {
				return uerr
			}
			inTok, outTok, haveUsage = in, out, true
		}
		if str(obj["type"]) != "message_end" || str(msg["role"]) != "assistant" {
			return nil
		}
		if t := piContentText(msg); t != "" {
			text, haveText = t, true
		}
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}
	if !haveText {
		return nil, 0, 0, &EnvelopeError{Code: storage.ProviderErrInvalidEnvelope, Err: errors.New("no assistant message_end")}
	}
	if !haveUsage {
		return nil, 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageMissing, Err: errors.New("no usage")}
	}
	return singleJSONObject(text, inTok, outTok)
}

func parseCodexJSONL(raw []byte) ([]byte, int64, int64, error) {
	var completed, fallback string
	var haveCompleted, haveFallback, haveUsage bool
	var inTok, outTok int64
	err := eachJSONL(raw, func(obj map[string]any) error {
		switch str(obj["type"]) {
		case "item.completed":
			item, _ := obj["item"].(map[string]any)
			if str(item["type"]) == "agent_message" {
				if t := firstString(item, "text", "content"); t != "" {
					completed, haveCompleted = t, true
				}
			}
		case "item.agent_message":
			if t := firstString(obj, "text", "content"); t != "" {
				fallback, haveFallback = t, true
			}
		case "turn.completed":
			if usage, ok := obj["usage"]; ok {
				in, out, uerr := readUsage(usage, false)
				if uerr != nil {
					return uerr
				}
				inTok, outTok, haveUsage = in, out, true
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}
	text := completed
	if !haveCompleted {
		text = fallback
	}
	if !haveCompleted && !haveFallback {
		return nil, 0, 0, &EnvelopeError{Code: storage.ProviderErrInvalidEnvelope, Err: errors.New("no agent message")}
	}
	if !haveUsage {
		return nil, 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageMissing, Err: errors.New("no turn.completed usage")}
	}
	return singleJSONObject(text, inTok, outTok)
}

func eachJSONL(raw []byte, fn func(map[string]any) error) error {
	if !utf8.Valid(raw) {
		return &EnvelopeError{Code: storage.ProviderErrInvalidEnvelope, Err: errors.New("stdout is not valid UTF-8")}
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if err := schema.RejectDuplicateKeys(line); err != nil {
			return &EnvelopeError{Code: storage.ProviderErrInvalidEnvelope, Err: err}
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return &EnvelopeError{Code: storage.ProviderErrInvalidEnvelope, Err: err}
		}
		if err := fn(obj); err != nil {
			return err
		}
	}
	return nil
}

func piContentText(msg map[string]any) string {
	items, ok := msg["content"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, item := range items {
		part, ok := item.(map[string]any)
		if !ok || str(part["type"]) != "text" {
			continue
		}
		b.WriteString(str(part["text"]))
	}
	return b.String()
}

func readUsage(usage any, shortNames bool) (int64, int64, error) {
	m, ok := usage.(map[string]any)
	if !ok {
		return 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageInvalid, Err: errors.New("usage is not an object")}
	}
	in := tokenField(m, "input_tokens")
	out := tokenField(m, "output_tokens")
	if shortNames {
		if in.missing {
			in = tokenField(m, "input")
		}
		if out.missing {
			out = tokenField(m, "output")
		}
	}
	if in.invalid || out.invalid {
		return 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageInvalid, Err: errors.New("usage counter invalid")}
	}
	if in.missing || out.missing {
		return 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageMissing, Err: errors.New("usage counter missing")}
	}
	if in.val < 0 || out.val < 0 {
		return 0, 0, &EnvelopeError{Code: storage.ProviderErrUsageInvalid, Err: errors.New("negative token count")}
	}
	return in.val, out.val, nil
}

type tokenFieldResult struct {
	val     int64
	missing bool
	invalid bool
}

func tokenField(m map[string]any, key string) tokenFieldResult {
	v, ok := m[key]
	if !ok {
		return tokenFieldResult{missing: true}
	}
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return tokenFieldResult{invalid: true}
		}
		return tokenFieldResult{val: int64(n)}
	default:
		return tokenFieldResult{invalid: true}
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := str(m[key]); s != "" {
			return s
		}
	}
	return ""
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
