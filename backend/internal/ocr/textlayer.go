package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TextLayer reads PDFs that already contain text, using poppler's pdftotext.
//
// This is the whole OCR story of the beta, and it is free in every sense: the
// binaries are in every distribution (GPL-2/3), there is no model, no pod, no
// daemon and no memory cost beyond a short-lived process. It answers nothing at
// all for photographs - and says so, rather than returning empty text and
// letting the caller assume the document was blank.
type TextLayer struct {
	// Paths to the poppler tools; injectable so tests do not need them
	// installed and so an image can place them anywhere.
	PdfToText string
	PdfInfo   string
	Limits    Limits
}

func NewTextLayer() *TextLayer {
	return &TextLayer{PdfToText: "pdftotext", PdfInfo: "pdfinfo", Limits: DefaultLimits()}
}

func (t *TextLayer) Name() string { return "textlayer" }

// minCharsPerPage is the same threshold the analysis script uses. Below it a
// PDF has a few stray characters from a stamp or a header, which is not a text
// layer anyone can work with.
const minCharsPerPage = 100

var pageCount = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)

func (t *TextLayer) Extract(ctx context.Context, document []byte, contentType string) (Result, error) {
	started := time.Now()
	result := Result{Provider: t.Name()}

	if !isPDF(document, contentType) {
		// Images are the common case here and they are not an error: they are
		// exactly what the deferred engine is for.
		result.Outcome = OutcomeUnsupported
		result.Note = "Kein PDF. Bilder brauchen eine Erkennungs-Engine (HF-0084)."
		result.Took = time.Since(started)
		return result, nil
	}
	if limit := t.Limits.MaxBytes; limit > 0 && int64(len(document)) > limit {
		result.Outcome = OutcomeRejected
		result.Note = fmt.Sprintf("Dokument ist %d Bytes groß, erlaubt sind %d.", len(document), limit)
		result.Took = time.Since(started)
		return result, ErrTooLarge
	}

	timeout := t.Limits.MaxDuration
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Page count first: it is cheap, and it is the limit that has to hold
	// before any content is read.
	pages, err := t.pages(ctx, document)
	if err != nil {
		result.Outcome = OutcomeUnsupported
		result.Note = "PDF nicht lesbar."
		result.Took = time.Since(started)
		return result, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	result.Pages = pages
	if limit := t.Limits.MaxPages; limit > 0 && pages > limit {
		result.Outcome = OutcomeRejected
		result.Note = fmt.Sprintf("Dokument hat %d Seiten, erlaubt sind %d.", pages, limit)
		result.Took = time.Since(started)
		return result, ErrTooLarge
	}

	text, err := t.text(ctx, document)
	if err != nil {
		result.Outcome = OutcomeUnsupported
		result.Note = "Text konnte nicht gelesen werden."
		result.Took = time.Since(started)
		return result, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	result.Text = strings.TrimSpace(text)
	result.Chars = len(strings.Join(strings.Fields(result.Text), ""))
	result.Took = time.Since(started)

	if pages > 0 && result.Chars/pages >= minCharsPerPage {
		result.Outcome = OutcomeText
		return result, nil
	}
	// Readable, but nothing to read. The measurement in HF-0073 says this is
	// the normal case for receipts.
	result.Outcome = OutcomeNeedsEngine
	result.Text = ""
	result.Note = fmt.Sprintf("Nur %d Zeichen auf %d Seiten - vermutlich ein Scan oder Foto.", result.Chars, pages)
	return result, nil
}

func (t *TextLayer) pages(ctx context.Context, document []byte) (int, error) {
	out, err := t.run(ctx, t.PdfInfo, document, "-")
	if err != nil {
		return 0, err
	}
	match := pageCount.FindSubmatch(out)
	if match == nil {
		return 0, errors.New("keine Seitenzahl gefunden")
	}
	return strconv.Atoi(string(match[1]))
}

func (t *TextLayer) text(ctx context.Context, document []byte) (string, error) {
	out, err := t.run(ctx, t.PdfToText, document, "-q", "-", "-")
	return string(out), err
}

// run pipes the document in through stdin and never writes it to disk: the
// bytes are untrusted, and a temporary file is one more thing to leak or to
// forget to delete.
func (t *TextLayer) run(ctx context.Context, binary string, document []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = bytes.NewReader(document)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", binary, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func isPDF(document []byte, contentType string) bool {
	if strings.HasPrefix(contentType, "application/pdf") {
		return true
	}
	return bytes.HasPrefix(document, []byte("%PDF-"))
}
