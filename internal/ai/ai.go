package ai

import "fmt"

// Generator generates a one-sentence summary for a paper. It is a placeholder
// for the future AI integration (phase 2).
type Generator interface {
	Summarize(title, authors, abstract string) (string, error)
}

// Noop is used when no AI service is configured.
type Noop struct{}

func (Noop) Summarize(title, authors, abstract string) (string, error) {
	return "", fmt.Errorf("AI summarization is not configured yet")
}

// Factory returns the configured generator. Extend here to add OpenAI-compatible
// or local-model integrations later.
func Factory() Generator {
	return Noop{}
}
