package pdf

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"runtime"
	"time"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// sealDataURI is the Alibi verification seal, inlined because Chrome's
// PrintToPDF footer template does not load external resources — an <img> with
// a file path or URL renders as a broken image. Source: assets/brand/alibi-seal.svg
const sealDataURI = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCA5NiA5NiI+PGcgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjOWFhMWFjIj48Y2lyY2xlIGN4PSI0OCIgY3k9IjQ4IiByPSI0NSIgc3Ryb2tlLXdpZHRoPSIyLjUiLz48Y2lyY2xlIGN4PSI0OCIgY3k9IjQ4IiByPSIzOSIgc3Ryb2tlLXdpZHRoPSIxIi8+PGcgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoNDggNDgpIHNjYWxlKC43MikgdHJhbnNsYXRlKC0zMiAtMzIpIj48ZyBzdHJva2Utd2lkdGg9IjQuNSIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIj48cGF0aCBkPSJNMjguOSAxOS4zIDE1LjIgNDYuNiIvPjxwYXRoIGQ9Ik0zNS4xIDE5LjMgNDguOCA0Ni42Ii8+PHBhdGggZD0iTTIwIDM3aDI0Ii8+PC9nPjxjaXJjbGUgY3g9IjIwIiBjeT0iMzciIHI9IjIuOCIgZmlsbD0iIzlhYTFhYyIgc3Ryb2tlPSJub25lIi8+PGNpcmNsZSBjeD0iNDQiIGN5PSIzNyIgcj0iMi44IiBmaWxsPSIjOWFhMWFjIiBzdHJva2U9Im5vbmUiLz48Y2lyY2xlIGN4PSIxMyIgY3k9IjUxIiByPSIzLjQiIHN0cm9rZS13aWR0aD0iMi40Ii8+PGNpcmNsZSBjeD0iNTEiIGN5PSI1MSIgcj0iMy40IiBzdHJva2Utd2lkdGg9IjIuNCIvPjxjaXJjbGUgY3g9IjMyIiBjeT0iMTMiIHI9IjUuNSIgZmlsbD0iI0M4OTEyRiIgc3Ryb2tlPSJub25lIi8+PC9nPjwvZz48L3N2Zz4="

func findChromiumBinary() string {
	// 1. Try PATH first
	names := []string{
		"msedge",
		"google-chrome",
		"chromium",
		"chromium-browser",
		"brave-browser",
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	// 2. OS-specific fallbacks
	switch runtime.GOOS {

	case "windows":
		candidates := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		}
		return firstExisting(candidates)

	case "darwin":
		candidates := []string{
			`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`,
			`/Applications/Brave Browser.app/Contents/MacOS/Brave Browser`,
			`/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge`,
		}
		return firstExisting(candidates)

	case "linux":
		candidates := []string{
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/usr/bin/brave-browser",
		}
		return firstExisting(candidates)
	}

	return ""
}

func firstExisting(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func generateWithChromium(html string) ([]byte, error) {
	bin := findChromiumBinary()
	if bin == "" {
		return nil, ErrChromiumNotFound
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(bin),
		chromedp.NoSandbox,
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// A long report with hundreds of activities takes real time to lay out.
	// The old 20s ceiling failed those before they finished rendering.
	ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var pdf []byte
	footerContent := `
		<div style="width: 100%; margin:0px 32px; font-size: 10px; font-weight: 400; display:flex; align-items:center; justify-content:space-between; color: #9aa1ac">
			<div style="display:flex; align-items:center">
				<img src="` + sealDataURI + `" style="width:18px; height:18px; display:block; margin-right:7px">
				<span>Generated on <span class="date"></span> with Alibi</span>
			</div>
			<div>Page <span class="pageNumber"></span> <span>/</span> <span class="totalPages"></span></div>
		</div>
	`
	// The caller hands over plain HTML; base64 for the data: URL is this
	// layer's concern. Note the ~2MB ceiling Chrome puts on data: navigation —
	// roughly 800 summarised commits — which is worth replacing with
	// SetDocumentContent if reports ever grow that large.
	encoded := base64.StdEncoding.EncodeToString([]byte(html))

	err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+encoded),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdf, _, err = page.PrintToPDF().WithPrintBackground(true).WithDisplayHeaderFooter(true).WithHeaderTemplate(" ").WithFooterTemplate(footerContent).WithMarginTop(0.5).WithMarginBottom(0.5).Do(ctx)
			return err
		}),
	)

	if err != nil {
		return nil, err
	}
	
	return pdf, nil
}
