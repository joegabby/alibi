package helpers

// import (
// 	"context"
// 	"fmt"
// 	"sort"
// 	"strings"

// 	"github.com/joegabby/alibi/internal/cli/types"
// )

// func renderHTMLReport(activities []types.Activity) string {
//     // sort by time if needed
//     sort.Slice(activities, func(i, j int) bool {
//         return activities[i].Timestamp.Before(activities[j].Timestamp)
//     })

//     var cards strings.Builder

//     for _, act := range activities {
//         if act.Type == "commit" {
//             cards.WriteString(renderCommitCard(act))
//         } else if act.Type == "push" {
//             cards.WriteString(renderPushCard(act))
//         }
//     }

//     return fmt.Sprintf(`
// <!DOCTYPE html>
// <html>
// <head>
//   <style>
//     body { font-family: Inter, Arial, sans-serif; }
//     .activity { page-break-inside: avoid; margin-bottom: 16px; }
//     ul { padding-left: 18px; }
//     footer { position: fixed; bottom: 0; font-size: 12px; }
//   </style>
// </head>
// <body>
//   <h1>Developer Activity Report</h1>
//   %s
// </body>
// </html>
// `, cards.String())
// }

// func generatePDF(html string) ([]byte, error) {
//     ctx, cancel := chromedp.NewContext(context.Background())
//     defer cancel()

//     var pdf []byte

//     err := chromedp.Run(ctx,
//         chromedp.Navigate("about:blank"),
//         chromedp.ActionFunc(func(ctx context.Context) error {
//             frameTree, err := page.GetFrameTree().Do(ctx)
//             if err != nil {
//                 return err
//             }
//             return page.SetDocumentContent(frameTree.Frame.ID, html).Do(ctx)
//         }),
//         chromedp.ActionFunc(func(ctx context.Context) error {
//             pdf, err = page.PrintToPDF().
//                 WithPrintBackground(true).
//                 WithPaperWidth(8.27).
//                 WithPaperHeight(11.7).
//                 WithMarginTop(0.5).
//                 WithMarginBottom(0.7).
//                 Do(ctx)
//             return err
//         }),
//     )

//     return pdf, err
// }

// func renderCommitCard(act types.Activity) string {
// 	var changesHTML strings.Builder

// 	for _, change := range act.FileChanges {
// 		var funcs strings.Builder

// 		for _, fn := range change.Functions {
// 			funcs.WriteString(fmt.Sprintf(
// 				`<li>%s %s (%d, %d)</li>`,
// 				fn.Action,
// 				fn.Name,
// 				fn.LineCounts[0],
// 				fn.LineCounts[1],
// 			))
// 		}

// 		changesHTML.WriteString(fmt.Sprintf(`
// <li>
//   <strong>%s %s</strong>
//   <ul>%s</ul>
// </li>`,
// 			change.Type,
// 			change.Path,
// 			funcs.String(),
// 		))
// 	}

// 	summary := ""
// 	if len(act.Commits) > 0 && act.Commits[0].Summary != nil {
// 		summary = act.Commits[0].Summary.Content
// 	} else {
// 		summary = fmt.Sprintf(`<ul>%s</ul>`, changesHTML.String())
// 	}

// 	return fmt.Sprintf(`
// <div class="activity commit-summary">
//   <div class="activity-meta">
//     <span class="badge badge-commit">%s</span>
//     <div class="activity-meta-summary">
//       <span>%s</span> |
//       <span>%d files</span>
//     </div>
//   </div>

//   <div class="activity-meta-summary">
//     <h3 class="msg">%s</h3>
//     <span class="id">%s</span>
//   </div>

//   <div class="summary">%s</div>
// </div>
// `,
// 		strings.ToUpper(act.Type),
// 		act.Commits[0].Timestamp.Format("02 Jan 2006 15:04"),
// 		len(act.Changes),
// 		act.Commits[0].Message,
// 		act.Commits[0].ID,
// 		summary,
// 	)
// }

// func renderPushCard(act Activity) string {
// 	var commits strings.Builder

// 	for _, c := range act.Commits {
// 		commits.WriteString(fmt.Sprintf(`
// <li>
//   <strong>%s</strong>
//   <div class="id">%s</div>
// </li>`, c.Message, c.ID))
// 	}

// 	return fmt.Sprintf(`
// <div class="activity push-summary">
//   <div class="activity-meta">
//     <span class="badge badge-push">PUSH</span>
//     <div class="activity-meta-summary">
//       <span>%s</span> |
//       <span>%d commits</span>
//     </div>
//   </div>

//   <ul>%s</ul>
// </div>
// `,
// 		act.Commits[0].Timestamp.Format("02 Jan 2006 15:04"),
// 		len(act.Commits),
// 		commits.String(),
// 	)
// }
