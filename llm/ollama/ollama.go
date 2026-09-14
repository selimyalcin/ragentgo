package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/selimyalcin/ragentgo/internal/retry"
	"github.com/selimyalcin/ragentgo/llm"
)

// Generator calls Ollama's /api/chat endpoint.
type Generator struct {
	baseURL string
	model   string
	http    *http.Client
	retry   retry.Policy
}

// Options configure the Ollama generator.
type Options struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewGenerator returns an Ollama chat generator.
func NewGenerator(opts Options) *Generator {
	if opts.BaseURL == "" {
		opts.BaseURL = "http://localhost:11434"
	}
	if opts.Model == "" {
		opts.Model = "llama3.1"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 120 * time.Second
	}
	return &Generator{
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		model:   opts.Model,
		http:    &http.Client{Timeout: opts.Timeout},
		retry:   retry.DefaultPolicy(),
	}
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// Generate implements llm.Generator.
func (g *Generator) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	payload, _ := json.Marshal(chatRequest{
		Model:    g.model,
		Messages: toChat(req),
		Stream:   false,
		Options:  optionsOf(req),
	})
	var raw []byte
	err := retry.Do(ctx, g.retry, func(ctx context.Context) error {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/api/chat", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := g.http.Do(httpReq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			return &retry.HTTPError{StatusCode: resp.StatusCode, Message: string(body)}
		}
		raw = body
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama generate: %w", err)
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ollama generate decode: %w", err)
	}
	return &llm.GenerateResponse{Text: parsed.Message.Content}, nil
}

// Stream implements llm.Generator.
func (g *Generator) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(ch)
		payload, err := json.Marshal(chatRequest{
			Model:    g.model,
			Messages: toChat(req),
			Stream:   true,
			Options:  optionsOf(req),
		})
		if err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/api/chat", bytes.NewReader(payload))
		if err != nil {
			ch <- llm.StreamChunk{Err: err}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := g.http.Do(httpReq)
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
		dec := json.NewDecoder(resp.Body)
		for {
			var piece chatResponse
			if err := dec.Decode(&piece); err != nil {
				if err != io.EOF {
					ch <- llm.StreamChunk{Err: err}
				} else {
					ch <- llm.StreamChunk{Done: true}
				}
				return
			}
			if piece.Message.Content != "" {
				select {
				case <-ctx.Done():
					ch <- llm.StreamChunk{Err: ctx.Err()}
					return
				case ch <- llm.StreamChunk{Text: piece.Message.Content}:
				}
			}
			if piece.Done {
				ch <- llm.StreamChunk{Done: true}
				return
			}
		}
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

func optionsOf(req llm.GenerateRequest) map[string]any {
	out := map[string]any{}
	if req.Temperature > 0 {
		out["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		out["num_predict"] = req.MaxTokens
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var _ llm.Generator = (*Generator)(nil)
