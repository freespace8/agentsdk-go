package anthropic

import "fmt"

const (
	defaultBaseURL     = "https://api.anthropic.com"
	messagesPath       = "/v1/messages"
	anthropicVersion   = "2023-06-01"
	defaultMaxTokens   = 1024
	defaultHTTPTimeout = 60 // seconds
	userAgent          = "agentsdk-go/anthropic"
)

// MessageRequest follows the Anthropic Messages API contract.
type MessageRequest struct {
	Model       string         `json:"model"`
	Messages    []MessageParam `json:"messages"`
	System      string         `json:"system,omitempty"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	TopK        *int           `json:"top_k,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// MessageParam represents a single conversational turn for Anthropic.
type MessageParam struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock is a union type for text, tool_use, and tool_result blocks.
type ContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
}

// MessageResponse captures the Anthropic message schema we care about.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence"`
}

// ErrorResponse models Anthropic error payloads.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody drills into the API error object.
type ErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// APIError surfaces Anthropic errors with HTTP metadata.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e APIError) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("anthropic API error (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("anthropic API error (%d, %s): %s", e.StatusCode, e.Type, e.Message)
}

// Stream event envelopes used by the SSE channel.
type StreamEventEnvelope struct {
	Type string `json:"type"`
}

// MessageStartEvent contains the first chunk with metadata.
type MessageStartEvent struct {
	Type    string          `json:"type"`
	Message MessageResponse `json:"message"`
}

// ContentBlockDeltaEvent yields incremental text.
type ContentBlockDeltaEvent struct {
	Type  string    `json:"type"`
	Index int       `json:"index"`
	Delta TextDelta `json:"delta"`
}

// TextDelta wraps the text emitted by a delta event.
type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MessageDeltaEvent communicates terminal metadata such as stop reasons.
type MessageDeltaEvent struct {
	Type  string       `json:"type"`
	Delta MessageDelta `json:"delta"`
}

// MessageDelta carries stop details.
type MessageDelta struct {
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

// MessageStopEvent terminates the stream.
type MessageStopEvent struct {
	Type string `json:"type"`
}
