package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/selimyalcin/ragentgo/internal/openaicompat"
	"github.com/selimyalcin/ragentgo/internal/retry"
	"github.com/selimyalcin/ragentgo/llm"
)

// Generator implements llm.Generator against an OpenAI-compatible chat API.
type Generator struct {
	client *openaicompat.Client
	model  string
}

// Options configure the generator.
type Options = openaicompat.Options

// NewGenerator returns an OpenAI-compatible chat generator.
func NewGenerator(opts Options) *Generator {
	if opts.Model == "" {
		opts.Model = "gpt-4.1-mini"
	}
	return &Generator{
		client: openaicompat.New(opts),
		model:  opts.Model,
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Generate implements llm.Generator.
func (g *Generator) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if err := g.client.Acquire(ctx); err != nil {
		return nil, err
	}
	defer g.client.Release()

	payload := chatRequest{
		Model:       g.model,
		Messages:    toChat(req),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	raw, err := g.client.Post(ctx, "/chat/completions", payload)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("generate: empty choices")
	}
	out := &llm.GenerateResponse{Text: resp.Choices[0].Message.Content}
	if resp.Usage != nil {
		out.Usage = llm.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	return out, nil
}

// Stream implements llm.Generator.
func (g *Generator) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(ch)
		if err := g.client.Acquire(ctx); err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		defer g.client.Release()

		payload := chatRequest{
			Model:       g.model,
			Messages:    toChat(req),
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			Stream:      true,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.client.BaseURL()+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		httpReq.Header.Set("Accept", "text/event-stream")
		g.client.WriteHeaders(httpReq)
		resp, err := g.client.HTTP().Do(httpReq)
		if err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(resp.Body)
			ch <- llm.StreamChunk{Err: &retry.HTTPError{StatusCode: resp.StatusCode, Message: string(raw)}}
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				ch <- llm.StreamChunk{Done: true}
				return
			}
			var piece struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &piece); err != nil {
				continue
			}
			if len(piece.Choices) == 0 {
				continue
			}
			text := piece.Choices[0].Delta.Content
			if text == "" {
				continue
			}
			select {
			case <-ctx.Done():
				ch <- llm.StreamChunk{Err: ctx.Err()}
				return
			case ch <- llm.StreamChunk{Text: text}:
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		ch <- llm.StreamChunk{Done: true}
	}()
	return ch, nil
}

func toChat(req llm.GenerateRequest) []chatMessage {
	msgs := llm.MessagesFromRequest(req)
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, chatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

var _ llm.Generator = (*Generator)(nil)
