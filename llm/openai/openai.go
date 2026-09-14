package openai

import (
	"github.com/selimyalcin/ragentgo/llm/openaicompat"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Generator is an OpenAI chat client using the public HTTP API.
type Generator = openaicompat.Generator

// Options configure the OpenAI generator.
type Options = openaicompat.Options

// NewGenerator returns a generator pointed at api.openai.com unless BaseURL is set.
func NewGenerator(opts Options) *Generator {
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	if opts.Model == "" {
		opts.Model = "gpt-4.1-mini"
	}
	return openaicompat.NewGenerator(opts)
}
