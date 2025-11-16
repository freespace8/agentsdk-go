package anthropic

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	modelpkg "github.com/cexll/agentsdk-go/pkg/model"
)

// Ensure AnthropicProvider satisfies the Provider interface at compile time.
var _ modelpkg.Provider = (*AnthropicProvider)(nil)

// AnthropicProvider wires Anthropic-backed model implementations into the
// factory.
type AnthropicProvider struct {
	HTTPClient *http.Client
}

// NewProvider builds an AnthropicProvider with the supplied HTTP client. When
// client is nil, a default client with sane timeouts will be used.
func NewProvider(client *http.Client) *AnthropicProvider {
	return &AnthropicProvider{HTTPClient: client}
}

// Name advertises the provider identifier used by the factory.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// NewModel materializes an AnthropicModel configured according to cfg.
func (p *AnthropicProvider) NewModel(ctx context.Context, cfg modelpkg.ModelConfig) (modelpkg.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("anthropic api key is required")
	}

	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(cfg.Name)
	}
	if modelName == "" {
		return nil, errors.New("anthropic model name is required")
	}

	baseURL := sanitizeBaseURL(cfg.BaseURL)
	headers := buildDefaultHeaders(apiKey)
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) == "" || v == "" {
			continue
		}
		headers[k] = v
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(defaultHTTPTimeout) * time.Second}
	}

	return &AnthropicModel{
		client:  client,
		baseURL: baseURL,
		model:   modelName,
		headers: headers,
		opts:    parseModelOptions(cfg.Extra),
	}, nil
}

func sanitizeBaseURL(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return defaultBaseURL
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return defaultBaseURL
	}
	return trimmed
}

func buildDefaultHeaders(apiKey string) map[string]string {
	return map[string]string{
		"X-API-Key":         apiKey,
		"Anthropic-Version": anthropicVersion,
		"Content-Type":      "application/json",
		"User-Agent":        userAgent,
	}
}
