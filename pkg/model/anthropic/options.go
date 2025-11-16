package anthropic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type modelOptions struct {
	MaxTokens   int
	Temperature *float64
	TopP        *float64
	TopK        *int
	System      string
	Metadata    map[string]any
}

func parseModelOptions(extra map[string]any) modelOptions {
	opts := modelOptions{MaxTokens: defaultMaxTokens}
	if len(extra) == 0 {
		return opts
	}
	for key, val := range extra {
		switch strings.ToLower(key) {
		case "max_tokens":
			if v, ok := toInt(val); ok {
				opts.MaxTokens = v
			}
		case "temperature":
			if v, ok := toFloat(val); ok {
				opts.Temperature = &v
			}
		case "top_p":
			if v, ok := toFloat(val); ok {
				opts.TopP = &v
			}
		case "top_k":
			if v, ok := toInt(val); ok {
				opts.TopK = &v
			}
		case "system":
			opts.System = fmt.Sprint(val)
		case "metadata":
			if m, ok := val.(map[string]any); ok {
				opts.Metadata = cloneMetadata(m)
			}
		}
	}
	return opts
}

func toInt(val any) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		return i, err == nil
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func toFloat(val any) (float64, bool) {
	switch v := val.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
