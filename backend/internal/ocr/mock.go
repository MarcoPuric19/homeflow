package ocr

import (
	"context"
	"strings"
	"time"
)

// Mock answers without touching any binary, so tests and a development machine
// without poppler still work. It is the second half of why the contract exists:
// nothing downstream should need a real engine to be testable.
type Mock struct {
	// Text is returned for anything that looks like a PDF.
	Text string
	// Err, if set, is returned instead - for exercising the failure path.
	Err error
	// Calls counts invocations, so a test can assert that a cache or a queue
	// did not process the same document twice.
	Calls int
}

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Extract(_ context.Context, document []byte, contentType string) (Result, error) {
	m.Calls++
	if m.Err != nil {
		return Result{Provider: m.Name(), Outcome: OutcomeUnsupported}, m.Err
	}
	if !isPDF(document, contentType) {
		return Result{Provider: m.Name(), Outcome: OutcomeUnsupported,
			Note: "Kein PDF."}, nil
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return Result{Provider: m.Name(), Outcome: OutcomeNeedsEngine, Pages: 1,
			Note: "Mock ohne hinterlegten Text."}, nil
	}
	return Result{
		Provider: m.Name(), Outcome: OutcomeText, Text: text, Pages: 1,
		Chars: len(strings.Join(strings.Fields(text), "")), Took: time.Millisecond,
	}, nil
}
