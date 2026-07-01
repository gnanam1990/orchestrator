package selector

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// maxRoutingTokens caps the completion: a routing answer is one short tool name
// or the literal "NONE", so a small budget is ample and keeps the call cheap.
const maxRoutingTokens = 100

// AnthropicCaller is an LLMCaller backed by the Anthropic Messages API.
//
// The underlying client reads credentials from ANTHROPIC_API_KEY (the first
// entry in the SDK's credential precedence chain), so no key is passed in code.
type AnthropicCaller struct {
	client anthropic.Client
	model  anthropic.Model
}

// NewAnthropicCaller returns a caller that routes with Claude Sonnet 4.6.
func NewAnthropicCaller() *AnthropicCaller {
	return &AnthropicCaller{
		client: anthropic.NewClient(), // picks up ANTHROPIC_API_KEY from the environment
		model:  anthropic.ModelClaudeSonnet4_6,
	}
}

// Complete sends the prompt as a single user message and returns the model's
// text response. The task is a short classification, so max_tokens is small and
// thinking is left off (Sonnet 4.6's default) for a fast, cheap call.
func (a *AnthropicCaller) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: maxRoutingTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: completion request failed: %w", err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	return text.String(), nil
}
