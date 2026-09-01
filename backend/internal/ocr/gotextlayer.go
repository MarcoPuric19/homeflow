package ocr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// GoTextLayer reads a PDF's text layer without leaving the process.
//
// This is what makes the text-layer path affordable in the beta. The
// alternative was poppler, which would mean giving up `FROM scratch` for an
// image with a shell in it, or standing up a second container. A library
// compiled into the binary costs neither - see ticket HF-0085 for the
// comparison and its limits.
//
// It answers nothing for photographs and says so, exactly like the poppler
// implementation.
type GoTextLayer struct {
	Limits Limits
}

func NewGoTextLayer() *GoTextLayer { return &GoTextLayer{Limits: DefaultLimits()} }

func (g *GoTextLayer) Name() string { return "go-textlayer" }

func (g *GoTextLayer) Extract(ctx context.Context, document []byte, contentType string) (Result, error) {
	started := time.Now()
	result := Result{Provider: g.Name()}
	finish := func(outcome Outcome, note string) Result {
		result.Outcome, result.Note, result.Took = outcome, note, time.Since(started)
		return result
	}

	if !isPDF(document, contentType) {
		return finish(OutcomeUnsupported, "Kein PDF. Bilder brauchen eine Erkennungs-Engine (HF-0084)."), nil
	}
	if limit := g.Limits.MaxBytes; limit > 0 && int64(len(document)) > limit {
		return finish(OutcomeRejected,
			fmt.Sprintf("Dokument ist %d Bytes groß, erlaubt sind %d.", len(document), limit)), ErrTooLarge
	}
	if err := ctx.Err(); err != nil {
		return finish(OutcomeUnsupported, "Abgebrochen."), err
	}

	text, pages, err := g.read(document)
	if err != nil {
		return finish(OutcomeUnsupported, "PDF nicht lesbar."), fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	result.Pages = pages
	if limit := g.Limits.MaxPages; limit > 0 && pages > limit {
		return finish(OutcomeRejected,
			fmt.Sprintf("Dokument hat %d Seiten, erlaubt sind %d.", pages, limit)), ErrTooLarge
	}
	result.Text = strings.TrimSpace(text)
	result.Chars = len(strings.Join(strings.Fields(result.Text), ""))

	if pages > 0 && result.Chars/pages >= minCharsPerPage {
		return finish(OutcomeText, ""), nil
	}
	result.Text = ""
	return finish(OutcomeNeedsEngine,
		fmt.Sprintf("Nur %d Zeichen auf %d Seiten - vermutlich ein Scan oder Foto.", result.Chars, pages)), nil
}

// read wraps the library call in a recover. That is not defensive habit: the
// input comes from outside the household, and this library uses panic for
// malformed structures. A panic here would take the whole API down.
func (g *GoTextLayer) read(document []byte) (text string, pages int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			text, pages = "", 0
			err = fmt.Errorf("beschädigtes PDF: %v", recovered)
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(document), int64(len(document)))
	if err != nil {
		return "", 0, err
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return "", reader.NumPage(), err
	}
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, plain); err != nil {
		return "", reader.NumPage(), err
	}
	return buffer.String(), reader.NumPage(), nil
}
