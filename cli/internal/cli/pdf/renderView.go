package pdf

import (
	"fmt"
	"strings"
	"time"

	"github.com/joegabby/alibi/cli/internal/cli/types"
)

type Function struct {
	Action     string
	Name       string
	LineCounts [2]int
}

type Change struct {
	Type      string
	Path      string
	Functions []Function
}

type Commit struct {
	ID        string
	Message   string
	Timestamp time.Time
	Summary   *Summary
}

type Summary struct {
	Content string
}

type Activity struct {
	Type    string
	Changes []Change
	Commits []Commit
}

// Helper to format time like JS fmtDate
func fmtDate(t string) string {
	// Try parsing common timestamp format: "2006-01-02T15:04:05Z07:00"
	parsed, err := time.Parse(time.RFC3339, t)
	if err != nil {
		// fallback: return original string if parsing fails
		return t
	}
	return parsed.Format("2006-01-02 15:04")
}

func renderCommitCard(act types.Activity) string {
	var rawChanges []string
	var functionCount int
	for _, change := range act.FileChanges {
		var functionsList []string
		if len(change.Functions) > 0 {
			for _, fn := range change.Functions {

				added, removed := "0", "0"
				if len(fn.LineCounts) >= 2 {
					added = fn.LineCounts[0]
					removed = fn.LineCounts[1]
				}
				functionCount++
				functionsList = append(functionsList, fmt.Sprintf(
					`<li class="fn-line">%s <code>%s</code> function (%s lines, %s lines)</li>`,
					fn.Action,
					fn.Name,
					added,
					removed,
				))
			}
		} else {
			functionsList = append(functionsList, "<p>No function modification recorded</p>")
		}

		rawChanges = append(rawChanges, fmt.Sprintf(`
<li>
  <div>
    <h3>%s %s</h3>
    <ul>%s</ul>
  </div>
</li>`, change.Type, change.Path, strings.Join(functionsList, "")))
	}
	content := ""
	if len(act.Commits) > 0 && act.Commits[0].Summary.Content != "" {
		content = fmt.Sprintf("<ul>%s</ul>", act.Commits[0].Summary.Content)
	} else if len(rawChanges) > 0 {
		content = fmt.Sprintf("<ul>%s</ul>", strings.Join(rawChanges, ""))
	} else {
		content = "<p class='small'>No changes recorded</p>"
	}

	return fmt.Sprintf(`
<div class="activity commit-summary">
  <div class="activity-header">
    <span class="badge badge-commit">%s</span>
    <div class="activity-meta-summary msg">
      <span>%s</span> |
      <span>%d files</span> |
      <span>%d functions</span>
    </div>
  </div>

  <div class="activity-meta-summary">
    <h3 class="msg">%s</h3>
    <span class="id">%s</span>
  </div>

  <div class="summary">
    %s
  </div>
</div>
`, strings.ToUpper(act.Type), fmtDate(act.Commits[0].Timestamp), len(act.FileChanges), functionCount, act.Commits[0].Message, act.Commits[0].ID, content)
}

func renderPushCard(act types.Activity) string {
	var commitsList []string
	for _, commit := range act.Commits {
		commitsList = append(commitsList, fmt.Sprintf(`
<li>
  <span class="activity-meta-summary">
    <h3 class="msg">%s</h3>
    <span class="id">%s</span>
  </span>
</li>`, commit.Message, commit.ID))
	}

	return fmt.Sprintf(`
<div class="activity push-summary">
  <div class="activity-header">
    <span class="badge badge-push">%s</span>
    <div class="activity-meta-summary msg">
      <span>%s</span> |
      <span>%d commits</span>
    </div>
  </div>

  <div class="summary">
    <ul>%s</ul>
  </div>
</div>
`, strings.ToUpper(act.Type), fmtDate(act.Commits[0].Timestamp), len(act.Commits), strings.Join(commitsList, ""))
}

func renderActivity(act types.Activity) string {
	switch strings.ToLower(act.Type) {
	case "commit":
		return renderCommitCard(act)
	case "push":
		return renderPushCard(act)
	default:
		return ""
	}
}

func RenderReport(activities []types.Activity, projectName, fromVal, toVal string) string {
	var totalCommits, totalPushes, totalFiles, totalFunctions int
	filePaths := make(map[string]struct{})
	functions := make(map[string]struct{})

	for _, act := range activities {
		switch strings.ToLower(act.Type) {
		case "commit":
			totalCommits++
		case "push":
			totalPushes++
		}

		for _, file := range act.FileChanges {
			filePaths[file.Path] = struct{}{}
			for _, function := range file.Functions {
				functions[file.Path+"::"+function.Name] = struct{}{}
			}
		}

	}
	totalFiles += len(filePaths)
	totalFunctions += len(functions)

	var activityCards []string
	for _, act := range activities {
		activityCards = append(activityCards, renderActivity(act))
	}

	html := fmt.Sprintf(`
	<html lang="en">

	<head>
	<meta charset="UTF-8" />
	<title>Developer Activity Report</title>

<style>
      /* PDF-safe reset */
      * {
        box-sizing: border-box;
		font-family: Inter, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial;
      }

      body {
        margin: 0;
        padding: 0px 32px;
        background: #ffffff;
        color: #111827;
        font-size: 12px;
        line-height: 1.6;
      }

      h1,
      h2,
      h3 {
        margin: 0 0 8px;
        font-weight: 600;
      }

      h1 {
        font-size: 22px;
      }

      h2 {
        font-size: 16px;
        margin-top: 24px;
      }

      h3 {
        font-size: 14px;
        margin-bottom: 4px;
      }
		code {
			font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, "Roboto Mono",
    "Courier New", monospace;
		}
      .muted {
        color: #6b7280;
      }

      /* Brand */
      .brand {
        display: flex;
        align-items: center;
        gap: 9px;
        margin-bottom: 14px;
      }

      .brand-mark {
        width: 26px;
        height: 26px;
        display: block;
        color: #16181d;
      }

      .brand-word {
        font-family: "Iowan Old Style", "Palatino Linotype", Palatino,
          "Book Antiqua", Georgia, serif;
        font-size: 15px;
        letter-spacing: 0.19em;
        line-height: 1;
        color: #16181d;
      }

      /* Header */
      .header {
        border-bottom: 2px solid #e5e7eb;
        padding-top: 8px;
        padding-bottom: 16px;
        margin-bottom: 24px;
      }

      .date-range {
        margin-top: 4px;
        font-size: 14px;
      }

      /* Stats grid */
      .stats {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: 12px;
        margin-bottom: 32px;
      }

      .stat {
        border: 1px solid #e5e7eb;
        border-radius: 6px;
        padding: 12px;
        text-align: center;
      }

      .stat-value {
        font-size: 18px;
        font-weight: 600;
        margin-bottom: 2px;
      }

      .stat-label {
        font-size: 11px;
        color: #6b7280;
      }

      /* Activity list */
      .activity {
        margin-bottom: 16px;
        border-left: 4px solid transparent;
        padding: 12px 16px;
        border-radius: 4px;
      }

      .activity-header {
        display: flex;
        gap: 12px;
        margin-bottom: 6px;
      }

      .activity-meta-summary {
        display: inline-flex;
        align-items: center;
        gap: 12px;
        font-size: 14px;
      }

      .activity-meta-summary .id {
        font-family: "Courier New", Courier, monospace;
        font-size: 14px;
      }

      /* Commit styling */
      .commit-summary {
	    background: #eff6ff;
		border-left-color: #3b82f6;

      }

      /* Push styling */
      .push-summary {
		background: #f0fdf4;
		border-left-color: #22c55e;

      }

      .badge {
        font-size: 10px;
        font-weight: 600;
        padding: 5px 10px;
        border-radius: 999px;
        color: white;
        display: flex;
        align-items: center;
        justify-items: center;
      }

      .badge-commit {
	    background: #2563eb;
      }

      .badge-push {
        background: #16a34a;
      }

      .summary {
        margin-top: 6px;
        padding-left: 16px;
		font-size: 14px;

      }

      .summary ul {
        margin: 4px 0 0;
        padding-left: 16px;
      }

      .summary li {
        margin-bottom: 4px;
      }

      /* Footer */
      .footer {
        margin-top: 40px;
        border-top: 1px solid #e5e7eb;
        padding-top: 12px;
        font-size: 10px;
        color: #6b7280;
        text-align: center;
      }

      /* Page breaks */
      .page-break {
        page-break-before: always;
      }
    </style>
	</head>
<body>
  <!-- HEADER -->
  <div class="header">
    <div class="brand">
      <svg class="brand-mark" viewBox="0 0 64 64" fill="none" role="img" aria-label="Alibi">
        <g stroke="currentColor" stroke-width="4.5" stroke-linecap="round">
          <path d="M28.9 19.3 15.2 46.6" />
          <path d="M35.1 19.3 48.8 46.6" />
          <path d="M20 37h24" />
        </g>
        <circle cx="20" cy="37" r="2.8" fill="currentColor" />
        <circle cx="44" cy="37" r="2.8" fill="currentColor" />
        <circle cx="13" cy="51" r="3.4" stroke="currentColor" stroke-width="2.4" />
        <circle cx="51" cy="51" r="3.4" stroke="currentColor" stroke-width="2.4" />
        <circle cx="32" cy="13" r="5.5" fill="#c8912f" />
      </svg>
      <span class="brand-word">ALIBI</span>
    </div>
    <h1>Developer Activity Report for %s</h1>
    <div class="date-range muted">
      Report Period: <strong>%s</strong> – <strong>%s</strong>
    </div>
  </div>

  <!-- STATS -->
  <div class="stats">
    <div class="stat">
      <div class="stat-value">%d</div>
      <div class="stat-label">Commits</div>
    </div>

    <div class="stat">
      <div class="stat-value">%d</div>
      <div class="stat-label">Pushes</div>
    </div>

    <div class="stat">
      <div class="stat-value">%d</div>
      <div class="stat-label">Files Affected</div>
    </div>

    <div class="stat">
      <div class="stat-value">%d</div>
      <div class="stat-label">Functions Affected</div>
    </div>
  </div>

  <!-- ACTIVITIES -->
  %s
</body>
</html>

`, projectName, fromVal, toVal, totalCommits, totalPushes, totalFiles, totalFunctions, strings.Join(activityCards, ""))

	// Returns plain HTML. Encoding belongs at the browser boundary, and the
	// caller needs the real markup so it can save the report as .html when no
	// browser is available to print it.
	return html
}
