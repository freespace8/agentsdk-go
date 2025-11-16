package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	modelpkg "github.com/cexll/agentsdk-go/pkg/model"
)

// Ensure AnthropicModel implements the Model interface.
var _ modelpkg.Model = (*AnthropicModel)(nil)

// AnthropicModel is a concrete model backed by Anthropic's Messages API.
type AnthropicModel struct {
	client  *http.Client
	baseURL string
	model   string
	headers map[string]string
	opts    modelOptions
}

// Generate performs a blocking Anthropic Messages API call.
func (m *AnthropicModel) Generate(ctx context.Context, messages []modelpkg.Message) (modelpkg.Message, error) {
	payload := m.buildPayload(messages, false)
	resp, err := m.doRequest(ctx, payload)
	if err != nil {
		return modelpkg.Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return modelpkg.Message{}, readAPIError(resp)
	}

	var msgResp MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return modelpkg.Message{}, fmt.Errorf("decode anthropic response: %w", err)
	}

	return convertResponse(msgResp), nil
}

// GenerateStream invokes the Anthropic streaming endpoint (SSE) and relays
// incremental chunks into cb.
func (m *AnthropicModel) GenerateStream(ctx context.Context, messages []modelpkg.Message, cb modelpkg.StreamCallback) error {
	if cb == nil {
		return errors.New("anthropic stream callback is required")
	}

	payload := m.buildPayload(messages, true)
	resp, err := m.doRequest(ctx, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return readAPIError(resp)
	}

	var full strings.Builder
	finalSent := false
	streamErr := consumeSSE(ctx, resp.Body, func(_ string, data string) error {
		data = strings.TrimSpace(data)
		if data == "" {
			return nil
		}

		var envelope StreamEventEnvelope
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			return fmt.Errorf("decode anthropic stream envelope: %w", err)
		}

		switch envelope.Type {
		case "content_block_delta":
			var delta ContentBlockDeltaEvent
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				return fmt.Errorf("decode anthropic delta: %w", err)
			}
			chunk := delta.Delta.Text
			if chunk == "" {
				return nil
			}
			full.WriteString(chunk)
			return cb(modelpkg.StreamResult{Message: modelpkg.Message{Role: "assistant", Content: chunk}})
		case "message_stop":
			if finalSent {
				return nil
			}
			finalSent = true
			return cb(modelpkg.StreamResult{
				Message: modelpkg.Message{Role: "assistant", Content: full.String()},
				Final:   true,
			})
		case "message_start", "content_block_start", "content_block_stop", "message_delta", "ping":
			return nil
		default:
			return nil
		}
	})

	if streamErr != nil {
		return streamErr
	}

	if !finalSent {
		return cb(modelpkg.StreamResult{
			Message: modelpkg.Message{Role: "assistant", Content: full.String()},
			Final:   true,
		})
	}

	return nil
}

func (m *AnthropicModel) buildPayload(messages []modelpkg.Message, stream bool) MessageRequest {
	systemText, chatMessages := toAnthropicMessages(messages)
	if m.opts.System != "" {
		if systemText != "" {
			systemText = systemText + "\n\n" + m.opts.System
		} else {
			systemText = m.opts.System
		}
	}

	payload := MessageRequest{
		Model:     m.model,
		Messages:  chatMessages,
		MaxTokens: m.opts.MaxTokens,
		Stream:    stream,
	}
	if payload.MaxTokens <= 0 {
		payload.MaxTokens = defaultMaxTokens
	}

	if systemText != "" {
		payload.System = systemText
	}
	if m.opts.Metadata != nil {
		payload.Metadata = cloneMetadata(m.opts.Metadata)
	}
	if m.opts.Temperature != nil {
		payload.Temperature = m.opts.Temperature
	}
	if m.opts.TopP != nil {
		payload.TopP = m.opts.TopP
	}
	if m.opts.TopK != nil {
		payload.TopK = m.opts.TopK
	}

	return payload
}

func (m *AnthropicModel) doRequest(ctx context.Context, payload MessageRequest) (*http.Response, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return nil, fmt.Errorf("encode anthropic request: %w", err)
	}

	endpoint := m.baseURL + messagesPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}

	for k, v := range m.headers {
		if v == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	return m.client.Do(req)
}

func readAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("anthropic api status %d: %w", resp.StatusCode, err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return APIError{StatusCode: resp.StatusCode, Message: resp.Status}
	}

	var apiErr ErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		return APIError{StatusCode: resp.StatusCode, Type: apiErr.Error.Type, Message: apiErr.Error.Message}
	}

	return APIError{StatusCode: resp.StatusCode, Message: string(body)}
}

func convertResponse(resp MessageResponse) modelpkg.Message {
	msg := modelpkg.Message{Role: resp.Role}
	var text strings.Builder
	var toolCalls []modelpkg.ToolCall
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, modelpkg.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}
	msg.Content = text.String()
	msg.ToolCalls = toolCalls
	if msg.Role == "" {
		msg.Role = "assistant"
	}
	return msg
}

func toAnthropicMessages(messages []modelpkg.Message) (string, []MessageParam) {
	var systemParts []string
	out := make([]MessageParam, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" {
			if msg.Content != "" {
				systemParts = append(systemParts, msg.Content)
			}
			continue
		}

		blocks := make([]ContentBlock, 0, 1+len(msg.ToolCalls))
		if msg.Content != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: msg.Content})
		}
		for _, call := range msg.ToolCalls {
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				ID:    call.ID,
				Name:  call.Name,
				Input: call.Arguments,
			})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, ContentBlock{Type: "text", Text: ""})
		}

		out = append(out, MessageParam{Role: normalizeRole(role), Content: blocks})
	}

	if len(out) == 0 {
		out = append(out, MessageParam{
			Role:    "user",
			Content: []ContentBlock{{Type: "text", Text: ""}},
		})
	}
	return strings.Join(systemParts, "\n\n"), out
}

func normalizeRole(role string) string {
	switch role {
	case "assistant", "model":
		return "assistant"
	default:
		return "user"
	}
}
