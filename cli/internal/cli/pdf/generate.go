package pdf

import (
	"errors"
	"fmt"
	"time"
)

// GeneratePDF renders the report HTML to PDF bytes using a headless browser.
//
// There is deliberately no second, pure-Go renderer behind this. The report's
// meaning lives in its CSS — the letterhead, the stat grid, the coloured
// badges — and a drawing API such as gofpdf has no layout engine to reproduce
// any of it. The honest fallback is to hand over the HTML report itself, which
// opens and prints from any browser; serve.go does that when this returns
// ErrChromiumNotFound.
func GeneratePDF(html string) ([]byte, error) {
	if !chromiumAvailable() {
		return nil, ErrChromiumNotFound
	}

	pdf, err := generateWithChromium(html)
	if err != nil {
		return nil, fmt.Errorf("could not render the report to PDF: %w", err)
	}
	if len(pdf) == 0 {
		return nil, errors.New("the browser returned an empty PDF")
	}

	return pdf, nil
}

func chromiumAvailable() bool {
	return findChromiumBinary() != ""
}

// ReportFileName builds a unique, filesystem-safe name for a saved report.
// ext is given without a leading dot, e.g. "pdf" or "html".
func ReportFileName(projectName, startDate, endDate, ext string) string {
	// Parse start date
	start, err := time.Parse(time.RFC3339, startDate)
	if err != nil {
		start = time.Now() // fallback to now
	}

	// Parse end date
	end, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		end = time.Now() // fallback to now
	}

	timestamp := time.Now().Unix() // unique seconds since epoch

	return fmt.Sprintf("%s_%s_%s_%d.%s",
		projectName,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
		timestamp,
		ext,
	)
}

var ErrChromiumNotFound = errors.New(
	"no compatible browser found — install Microsoft Edge, Google Chrome, Chromium, or Brave to export PDFs")
