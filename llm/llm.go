package llm

import "context"

// Message is a single chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateRequest is a non-streaming completion request.
type GenerateRequest struct {
	Messages    []Message
	System      string
	Temperature float32
	MaxTokens   int
}

// Usage captures token accounting when the provider reports it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GenerateResponse is a completed generation.
type GenerateResponse struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
}

// StreamChunk is one token (or fragment) of a streamed completion.
type StreamChunk struct {
	Text  string `json:"text"`
	Done  bool   `json:"done"`
	Usage Usage  `json:"usage,omitempty"`
	Err   error  `json:"-"`
}

// Generator produces model completions.
type Generator interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)
}

// MessagesFromRequest flattens System + Messages into a chat transcript.
func MessagesFromRequest(req GenerateRequest) []Message {
	out := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		out = append(out, Message{Role: "system", Content: req.System})
	}
	out = append(out, req.Messages...)
	return out
}
