package ocr

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func provider(t *testing.T) *TextLayer {
	t.Helper()
	for _, binary := range []string{"pdftotext", "pdfinfo"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s nicht installiert", binary)
		}
	}
	return NewTextLayer()
}

func TestPdfWithATextLayerIsRead(t *testing.T) {
	result, err := provider(t).Extract(context.Background(), read(t, "mit-textschicht.pdf"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeText {
		t.Fatalf("ergebnis=%q hinweis=%q", result.Outcome, result.Note)
	}
	if !strings.Contains(result.Text, "Tierarztpraxis") || !strings.Contains(result.Text, "112,40") {
		t.Fatalf("text=%q", result.Text)
	}
	if result.Pages != 1 || result.Chars < 100 {
		t.Fatalf("seiten=%d zeichen=%d", result.Pages, result.Chars)
	}
}

func TestScannedPdfIsReportedAsNeedingAnEngine(t *testing.T) {
	// Der Normalfall laut Messung in HF-0073. Wichtig ist, dass das NICHT als
	// leeres Dokument durchgeht - sonst hielte der Aufrufer den Beleg für leer.
	result, err := provider(t).Extract(context.Background(), read(t, "gescannt.pdf"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeNeedsEngine {
		t.Fatalf("ergebnis=%q", result.Outcome)
	}
	if result.Text != "" {
		t.Fatalf("Text zurückgegeben, obwohl keiner erkannt wurde: %q", result.Text)
	}
	if result.Note == "" {
		t.Fatal("ohne Begründung")
	}
}

func TestImagesAreUnsupportedNotBroken(t *testing.T) {
	result, err := provider(t).Extract(context.Background(), read(t, "foto.jpg"), "image/jpeg")
	if err != nil {
		t.Fatalf("ein Foto ist kein Fehler: %v", err)
	}
	if result.Outcome != OutcomeUnsupported {
		t.Fatalf("ergebnis=%q", result.Outcome)
	}
	if !strings.Contains(result.Note, "HF-0084") {
		t.Fatalf("Hinweis nennt das zuständige Ticket nicht: %q", result.Note)
	}
}

func TestContentTypeIsNotTrustedOnItsOwn(t *testing.T) {
	// Ein JPEG, das sich als PDF ausgibt, darf nicht in den PDF-Pfad geraten,
	// und ein PDF ohne passenden Content-Type soll trotzdem gelesen werden.
	if result, _ := provider(t).Extract(context.Background(), read(t, "foto.jpg"), "application/pdf"); result.Outcome == OutcomeText {
		t.Fatal("als PDF deklariertes JPEG wurde als Text gelesen")
	}
	result, err := provider(t).Extract(context.Background(), read(t, "mit-textschicht.pdf"), "")
	if err != nil || result.Outcome != OutcomeText {
		t.Fatalf("PDF ohne Content-Type: ergebnis=%q err=%v", result.Outcome, err)
	}
}

func TestBrokenPdfFailsLoudly(t *testing.T) {
	result, err := provider(t).Extract(context.Background(), read(t, "kaputt.pdf"), "application/pdf")
	if !errors.Is(err, ErrUnreadable) {
		t.Fatalf("err=%v", err)
	}
	if result.Outcome != OutcomeUnsupported {
		t.Fatalf("ergebnis=%q", result.Outcome)
	}
}

func TestLimitsAreCheckedBeforeAnyWork(t *testing.T) {
	p := provider(t)
	p.Limits = Limits{MaxBytes: 100, MaxPages: 50, MaxDuration: time.Second}
	result, err := p.Extract(context.Background(), read(t, "mit-textschicht.pdf"), "application/pdf")
	if !errors.Is(err, ErrTooLarge) || result.Outcome != OutcomeRejected {
		t.Fatalf("ergebnis=%q err=%v", result.Outcome, err)
	}
	if !strings.Contains(result.Note, "Bytes") {
		t.Fatalf("hinweis=%q", result.Note)
	}

}

func TestPageLimitRefusesLongDocuments(t *testing.T) {
	p := provider(t)
	two := read(t, "zwei-seiten.pdf")

	// Genau am Limit muss durchgehen.
	p.Limits = Limits{MaxBytes: 10 << 20, MaxPages: 2, MaxDuration: time.Second}
	result, err := p.Extract(context.Background(), two, "application/pdf")
	if err != nil || result.Outcome != OutcomeText || result.Pages != 2 {
		t.Fatalf("am Limit: ergebnis=%q seiten=%d err=%v", result.Outcome, result.Pages, err)
	}

	// Eine Seite darüber nicht - und die Ablehnung nennt die Zahlen.
	p.Limits.MaxPages = 1
	result, err = p.Extract(context.Background(), two, "application/pdf")
	if !errors.Is(err, ErrTooLarge) || result.Outcome != OutcomeRejected {
		t.Fatalf("über dem Limit: ergebnis=%q err=%v", result.Outcome, err)
	}
	if !strings.Contains(result.Note, "2 Seiten") || result.Text != "" {
		t.Fatalf("hinweis=%q text=%q", result.Note, result.Text)
	}

	// Null oder weniger heißt unbegrenzt.
	p.Limits.MaxPages = 0
	if result, err := p.Extract(context.Background(), two, "application/pdf"); err != nil || result.Outcome != OutcomeText {
		t.Fatalf("unbegrenzt: ergebnis=%q err=%v", result.Outcome, err)
	}
}

func TestCancellationIsRespected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider(t).Extract(ctx, read(t, "mit-textschicht.pdf"), "application/pdf")
	if err == nil {
		t.Fatal("abgebrochener Kontext wurde ignoriert")
	}
}

func TestTheDocumentIsNeverModified(t *testing.T) {
	original := read(t, "mit-textschicht.pdf")
	copied := append([]byte(nil), original...)
	if _, err := provider(t).Extract(context.Background(), copied, "application/pdf"); err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(original) {
		t.Fatal("Eingabe wurde verändert")
	}
}

func TestMockNeedsNoBinaries(t *testing.T) {
	mock := &Mock{Text: "Rechnung 112,40 EUR"}
	result, err := mock.Extract(context.Background(), []byte("%PDF-1.4 egal"), "application/pdf")
	if err != nil || result.Outcome != OutcomeText || result.Chars == 0 {
		t.Fatalf("ergebnis=%#v err=%v", result, err)
	}
	if _, err := mock.Extract(context.Background(), []byte("kein pdf"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if mock.Calls != 2 {
		t.Fatalf("aufrufe=%d", mock.Calls)
	}
	failing := &Mock{Err: errors.New("kaputt")}
	if _, err := failing.Extract(context.Background(), []byte("%PDF-"), "application/pdf"); err == nil {
		t.Fatal("Fehlerpfad des Mocks greift nicht")
	}
}

// Der Vertrag muss von beiden Implementierungen erfüllt werden - sonst ist er
// keiner.
func TestBothImplementationsSatisfyTheContract(t *testing.T) {
	var providers []Provider = []Provider{&Mock{}, NewTextLayer()}
	for _, p := range providers {
		if p.Name() == "" {
			t.Fatalf("%T ohne Namen", p)
		}
	}
}

// Die reine Go-Umsetzung muss sich genauso verhalten wie die über poppler -
// sonst ist der Vertrag keiner. Deshalb dieselben Vorlagen, dieselben
// Erwartungen.
func TestGoTextLayerBehavesLikeThePopplerOne(t *testing.T) {
	g := NewGoTextLayer()

	result, err := g.Extract(context.Background(), read(t, "mit-textschicht.pdf"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeText {
		t.Fatalf("ergebnis=%q hinweis=%q", result.Outcome, result.Note)
	}
	// Wortgrenzen sind der Grund, warum diese Bibliothek gewählt wurde und
	// nicht rsc.io/pdf: dort kleben die Wörter zusammen.
	if !strings.Contains(result.Text, "Tierarztpraxis Dr. Sommer") {
		t.Fatalf("Leerzeichen gehen verloren: %q", result.Text)
	}
	if !strings.Contains(result.Text, "112,40") {
		t.Fatalf("Betrag fehlt: %q", result.Text)
	}

	if result, err := g.Extract(context.Background(), read(t, "gescannt.pdf"), "application/pdf"); err != nil || result.Outcome != OutcomeNeedsEngine {
		t.Fatalf("Scan: ergebnis=%q err=%v", result.Outcome, err)
	}
	if result, err := g.Extract(context.Background(), read(t, "foto.jpg"), "image/jpeg"); err != nil || result.Outcome != OutcomeUnsupported {
		t.Fatalf("Foto: ergebnis=%q err=%v", result.Outcome, err)
	}
	if _, err := g.Extract(context.Background(), read(t, "kaputt.pdf"), "application/pdf"); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("kaputt: err=%v", err)
	}
}

func TestGoTextLayerCountsPagesAndHonoursLimits(t *testing.T) {
	g := NewGoTextLayer()
	result, err := g.Extract(context.Background(), read(t, "zwei-seiten.pdf"), "application/pdf")
	if err != nil || result.Pages != 2 || result.Outcome != OutcomeText {
		t.Fatalf("ergebnis=%q seiten=%d err=%v", result.Outcome, result.Pages, err)
	}
	g.Limits = Limits{MaxBytes: 10 << 20, MaxPages: 1}
	if result, err := g.Extract(context.Background(), read(t, "zwei-seiten.pdf"), "application/pdf"); !errors.Is(err, ErrTooLarge) || result.Outcome != OutcomeRejected {
		t.Fatalf("Seitenlimit: ergebnis=%q err=%v", result.Outcome, err)
	}
	g.Limits = Limits{MaxBytes: 100}
	if result, err := g.Extract(context.Background(), read(t, "zwei-seiten.pdf"), "application/pdf"); !errors.Is(err, ErrTooLarge) || result.Outcome != OutcomeRejected {
		t.Fatalf("Bytelimit: ergebnis=%q err=%v", result.Outcome, err)
	}
}

func TestGoTextLayerSurvivesGarbageWithoutPanicking(t *testing.T) {
	// Die Eingabe kommt von außerhalb des Haushalts. Ein Absturz hier nähme die
	// ganze API mit.
	g := NewGoTextLayer()
	for name, document := range map[string][]byte{
		"leer":              {},
		"nur Kopfzeile":     []byte("%PDF-1.4"),
		"abgeschnitten":     append([]byte("%PDF-1.4\n"), read(t, "mit-textschicht.pdf")[:200]...),
		"zufälliger Inhalt": bytes.Repeat([]byte{0x00, 0xff, 0x41}, 500),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := g.Extract(context.Background(), document, "application/pdf")
			if err == nil && result.Outcome == OutcomeText {
				t.Fatalf("Müll wurde als Text gelesen: %#v", result)
			}
		})
	}
}

func TestBothTextLayerProvidersAgree(t *testing.T) {
	// Wenn poppler vorhanden ist, müssen beide dieselbe Einstufung liefern.
	for _, binary := range []string{"pdftotext", "pdfinfo"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s nicht installiert", binary)
		}
	}
	popplerBased := NewTextLayer()
	goBased := NewGoTextLayer()
	for _, name := range []string{"mit-textschicht.pdf", "zwei-seiten.pdf", "gescannt.pdf", "foto.jpg"} {
		document := read(t, name)
		contentType := "application/pdf"
		if strings.HasSuffix(name, ".jpg") {
			contentType = "image/jpeg"
		}
		left, _ := popplerBased.Extract(context.Background(), document, contentType)
		right, _ := goBased.Extract(context.Background(), document, contentType)
		if left.Outcome != right.Outcome {
			t.Fatalf("%s: poppler=%q go=%q", name, left.Outcome, right.Outcome)
		}
		if left.Pages != right.Pages {
			t.Fatalf("%s: Seiten poppler=%d go=%d", name, left.Pages, right.Pages)
		}
	}
}
