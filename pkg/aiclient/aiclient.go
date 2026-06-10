// Package aiclient is a thin, mockable transport over an OpenAI-compatible LLM
// (spec §8). The production client wraps any-llm-go's OpenAI provider configured
// from config.AI (endpoint/key/model — no hardcoded vendor). Rate limiting,
// singleflight, and timeout handling live in pkg/rewrite, not here.
package aiclient

import (
	"context"
	"fmt"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	"github.com/eli-yip/rss-ai/pkg/config"
)

// Client turns a system+user prompt into the model's text reply.
type Client interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// completer is the subset of an any-llm provider this package uses.
type completer interface {
	Completion(ctx context.Context, params anyllm.CompletionParams) (*anyllm.ChatCompletion, error)
}

type openAIClient struct {
	provider completer
	model    string
}

// New builds an OpenAI-compatible client from config. The per-call timeout is
// supplied by the caller's context (pkg/rewrite), not here.
func New(cfg config.AI) (Client, error) {
	provider, err := openai.New(anyllm.WithBaseURL(cfg.BaseURL), anyllm.WithAPIKey(cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("init ai provider: %w", err)
	}
	return &openAIClient{provider: provider, model: cfg.Model}, nil
}

func (c *openAIClient) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := c.provider.Completion(ctx, anyllm.CompletionParams{
		Model: c.model,
		Messages: []anyllm.Message{
			{Role: anyllm.RoleSystem, Content: system},
			{Role: anyllm.RoleUser, Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("ai response has no choices")
	}
	// any-llm-go types Message.Content as any; a chat reply is a plain string.
	text, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("ai response content is not text (%T)", resp.Choices[0].Message.Content)
	}
	return strings.TrimSpace(text), nil
}
