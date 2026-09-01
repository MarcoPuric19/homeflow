// Package ocr extracts text from uploaded documents.
//
// The name is aspirational for now. Measurement in ticket HF-0073 showed that
// none of the household's receipts carry a text layer, and that a real OCR
// engine capable of reading both high-resolution scans and WhatsApp-compressed
// photos does not fit the laptop this runs on. So the beta ships the half that
// is free: documents that already contain text are read directly, everything
// else is reported as "needs an engine" and left alone.
//
// The contract below is the reason this package exists at all. An engine can be
// added later behind it without touching a single caller - that is ticket
// HF-0084.
package ocr

import (
	"context"
	"errors"
	"time"
)

// Outcome says what happened, so a caller never has to guess why a result is
// empty.
type Outcome string

const (
	// OutcomeText: the document carried a usable text layer and it was read.
	OutcomeText Outcome = "text"
	// OutcomeNeedsEngine: readable document, but no text to extract. This is
	// the honest answer during the beta, not an error.
	OutcomeNeedsEngine Outcome = "engine-noetig"
	// OutcomeUnsupported: not a format this provider handles at all.
	OutcomeUnsupported Outcome = "nicht-unterstuetzt"
	// OutcomeRejected: refused before doing any work - too large, too many
	// pages, too many pixels. A refusal is a result, not a failure.
	OutcomeRejected Outcome = "abgelehnt"
)

// Result of one extraction. Confidence is deliberately absent: a text layer is
// not recognised, it is read, and inventing a number for it would suggest a
// certainty that has no meaning here. An engine implementation adds it.
type Result struct {
	Outcome  Outcome       `json:"ergebnis"`
	Text     string        `json:"text,omitempty"`
	Pages    int           `json:"seiten,omitempty"`
	Chars    int           `json:"zeichen"`
	Provider string        `json:"anbieter"`
	Took     time.Duration `json:"-"`
	// Note explains a refusal or an empty result in one sentence, for the log
	// and for the person looking at the document.
	Note string `json:"hinweis,omitempty"`
}

// Limits are a control, not a budget - see docs/security/threat-model.md. They
// are checked before anything is decoded, because a small compressed file can
// expand into hundreds of megabytes.
type Limits struct {
	MaxBytes    int64
	MaxPages    int
	MaxDuration time.Duration
}

// DefaultLimits are sized from the documents actually measured in HF-0073:
// the largest was 2 pages at 21 MP. Anything far beyond that is not a household
// receipt and does not need to be processed by surprise.
func DefaultLimits() Limits {
	return Limits{
		MaxBytes:    10 << 20, // same ceiling the upload already enforces
		MaxPages:    50,
		MaxDuration: 30 * time.Second,
	}
}

var (
	ErrTooLarge   = errors.New("dokument überschreitet die Grenzen")
	ErrUnreadable = errors.New("dokument nicht lesbar")
)

// Provider is the seam. Everything downstream speaks to this and to nothing
// else, so replacing the beta implementation with a real engine later is a
// wiring change and not a rewrite.
type Provider interface {
	// Name identifies the implementation in logs and in results.
	Name() string
	// Extract reads what it can. It must respect ctx and must not modify the
	// input.
	Extract(ctx context.Context, document []byte, contentType string) (Result, error)
}
