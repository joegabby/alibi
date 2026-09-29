package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/joegabby/alibi/cli/internal/cli/constants"
	"github.com/joegabby/alibi/cli/internal/cli/types"
)

// ------- Configurable params -------
const (
	smallThreshold = 200 // lines considered "small" – tune as needed
	bgWorkerCount  = 3   // background workers for medium/large jobs

	// The only deadline that governs an LLM call. Six workers run at once, and
	// free-tier models (OpenRouter's ":free" ids in particular) queue requests,
	// so time-to-first-byte alone can exceed a minute under load.
	llmTimeout = 3 * time.Minute
)

// ------- SSE helpers -------
func SendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data any) error {
	var payload []byte
	switch d := data.(type) {
	case []byte:
		payload = d
	default:
		var err error
		payload, err = json.Marshal(d)
		if err != nil {
			return err
		}
	}
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	// Split by hand rather than with bufio.Scanner. Its 64KB token limit made
	// Scan() return false immediately for a large payload, and since
	// scanner.Err() went unchecked the event shipped with no data lines at all —
	// which the client then skipped, silently losing the whole message.
	for _, line := range bytes.Split(payload, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// ------- Worker function -------
func worker(ctx context.Context, id int, jobs <-chan types.Job, results chan<- types.SummaryResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		// Build the prompt
		prompt := `Generate a HTML list item (<li>…</li>) summarizing the key technical updates in the following code changes.

		- Each bullet must start with an action verb (Added, Updated, Refactored, Removed, Fixed, Improved).
		- Include only functional or structural changes affecting behavior, data, or architecture.
		- Wrap function names, variables, classes, tables, and columns in <code> tags.
		- Group related changes into single bullets when logical.
		- Focus on clarity, impact, and purpose.
		- Output only raw HTML, no markdown or extra text.
		`
		// 		prompt := `Generate a short executive summary describing the essential changes made in the following code updates.

		// - Use clear, concise, non-technical language suitable for leadership or stakeholders.
		// - Focus on the goals, improvements, and overall impact rather than detailed implementation steps.
		// - Group related updates into unified themes (e.g., authentication overhaul, database restructuring).
		// - Highlight outcomes, benefits, or reasons behind major changes.
		// - Avoid code-level terminology unless crucial; when necessary, wrap technical terms in <code> tags.
		// - Output only the executive summary text, without bullet points, markdown, or HTML tags.
		// `

		var summaryPrompt string
		summaryPrompt = fmt.Sprintf("%s\nCommit message: %s\nDiff:\n%s", prompt, job.CommitMsg, job.Diff)

		// Create context with timeout for the LLM call.
		// Released as soon as the call returns — a deferred cancel here would
		// hold every job's context alive until the whole worker exits.
		ctxTimeout, cancel := context.WithTimeout(ctx, llmTimeout)
		html, err := GetAISummary(ctxTimeout, summaryPrompt, job.Provider)
		cancel()

		res := types.SummaryResult{Path: job.Path, HTML: html}
		if err != nil {
			res.Err = err.Error()
		}
		// Send result or exit if context is canceled
		select {
		case results <- res:
		case <-ctx.Done():
			return
		}
	}
}

// ------- SSE endpoint handler -------
func SummaryStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}

	var data types.CommitData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		SendSSE(w, flusher, "error", err.Error())
		return
	}

	if len(data.Paths) < 1 {
		SendSSE(w, flusher, "error", types.SummaryResult{Path: "", Err: "No paths in request"})
	}

	for evt := range ProcessSummaries(r.Context(), data) {
		SendSSE(w, flusher, evt.Type, evt.Data)
	}
}

func ProcessSummaries(ctx context.Context, data types.CommitData) <-chan types.SummaryEvent {
	events := make(chan types.SummaryEvent)
	go func() {
		defer close(events)

		revList, err := RunGitInDir(
			data.Source, "rev-list", "--parents", "-n", "1", data.Hash,
		)
		if err != nil {
			events <- types.SummaryEvent{Type: "error", Data: err.Error()}
			return
		}

		isFirstCommit := len(strings.Fields(strings.TrimSpace(revList))) == 1

		var jobsAll []types.Job
		for _, path := range data.Paths {
			job, err := buildJob(data, path, isFirstCommit)
			if err != nil {
				events <- types.SummaryEvent{
					Type: "error",
					Data: types.SummaryResult{Path: path, Err: err.Error()},
				}
				continue
			}
			jobsAll = append(jobsAll, job)
		}

		// Every job shares one provider, so the first resolved model is the one
		// that actually ran. Record that rather than the possibly-empty value
		// the dashboard asked for.
		if len(jobsAll) > 0 {
			data.Model = jobsAll[0].Provider.Model
		}

		sortJobsBySizeAsc(jobsAll)

		smallJobs := make(chan types.Job, len(jobsAll))
		largeJobs := make(chan types.Job, len(jobsAll))
		results := make(chan types.SummaryResult, len(jobsAll))

		for _, j := range jobsAll {
			if j.Size <= smallThreshold {
				smallJobs <- j
			} else {
				largeJobs <- j
			}
		}
		close(smallJobs)
		close(largeJobs)

		var wg sync.WaitGroup

		startWorkers := func(jobs <-chan types.Job, n, startID int) {
			for i := 0; i < n; i++ {
				wg.Add(1)
				go worker(ctx, startID+i, jobs, results, &wg)
			}
		}

		startWorkers(smallJobs, 3, 1)
		startWorkers(largeJobs, bgWorkerCount, 10)

		go func() {
			wg.Wait()
			close(results)
		}()

		events <- types.SummaryEvent{
			Type: "started",
			Data: map[string]any{
				"total_files": len(jobsAll),
				"small_count": countJobsBySize(jobsAll, smallThreshold, true),
				"large_count": countJobsBySize(jobsAll, smallThreshold, false),
			},
		}

		allResults := make([]string, 0, len(jobsAll))

		for {
			select {
			case <-ctx.Done():
				events <- types.SummaryEvent{Type: "error", Data: "operation cancelled"}
				return

			case res, ok := <-results:
				if !ok {
					goto SAVE
				}

				if res.Err != "" {
					events <- types.SummaryEvent{Type: "error", Data: res}
					continue
				}

				allResults = append(allResults, res.HTML)
				events <- types.SummaryEvent{Type: "summary", Data: res}
			}
		}

	SAVE:
		content := strings.TrimSpace(strings.Join(allResults, ""))
		if content != "" {
			events <- types.SummaryEvent{Type: "saving", Data: "saving to file"}

			saved, err := saveData(ctx, allResults, data)
			if err != nil {
				events <- types.SummaryEvent{
					Type: "error",
					Data: "failed to save summaries: " + err.Error(),
				}
				return
			}

			// Carries provider, model and timestamp so callers can attach the
			// real record instead of rebuilding a bare one from the HTML.
			//
			// CONTRACT: Content is deliberately empty here. Every chunk was
			// already delivered via the "summary" events, and repeating the
			// whole text made the frame large enough to matter. Consumers must
			// merge this metadata with the chunks they collected — assigning
			// this record wholesale blanks the summary. The full text is on
			// disk in cache.json.
			meta := saved
			meta.Content = ""
			events <- types.SummaryEvent{Type: "saved", Data: meta}
		}

		events <- types.SummaryEvent{Type: "done", Data: "all summaries processed"}
	}()

	return events
}

func buildJob(data types.CommitData, path string, isFirst bool) (types.Job, error) {
	AIprovider, err := getAIProvider(data.Provider, data.Dir)
	if err != nil {
		return types.Job{}, fmt.Errorf("failed to retrieve AI provider: %w", err)
	}
	// The model comes from the dashboard, where empty means "use the provider
	// default". Resolve that to a concrete id here rather than leaving it to
	// GetAISummary, so the id that ran is the id we can record and display.
	AIprovider.Model = data.Model
	AIprovider.Model = resolveModel(AIprovider)
	if isFirst {
		content, err := RunGitInDir(
			data.Source, "show", fmt.Sprintf("%s:%s", data.Hash, path),
		)
		if err != nil {
			return types.Job{}, fmt.Errorf("git show failed: %w", err)
		}
		return types.Job{
			Path:       path,
			Diff:       content,
			Size:       countLines(content),
			CommitMsg:  data.Message,
			Provider:   AIprovider,
			CommitHash: data.Hash,
			RepoDir:    data.Source,
		}, nil
	}

	diff, err := RunGitInDir(
		data.Source,
		"diff",
		"--minimal",
		"--color-moved",
		"--unified=0",
		data.Hash+"^",
		data.Hash,
		"--",
		path,
	)
	if err != nil {
		return types.Job{}, fmt.Errorf("git diff failed: %w", err)
	}

	return types.Job{
		Path:       path,
		Diff:       diff,
		Size:       countLines(diff),
		CommitMsg:  data.Message,
		Provider:   AIprovider,
		CommitHash: data.Hash,
		RepoDir:    data.Source,
	}, nil
}

// saveData writes the summary into the project's cache file and returns the
// record it stored, so callers can display exactly what was persisted.
func saveData(ctx context.Context, allResults []string, data types.CommitData) (types.SummaryData, error) {
	select {
	case <-ctx.Done():
		return types.SummaryData{}, ctx.Err()
	default:
	}

	summaryData := types.SummaryData{
		CommitID:  data.Hash,
		Msg:       data.Message,
		Provider:  data.Provider,
		Model:     data.Model,
		TimeStamp: time.Now().Format(time.RFC3339),
		Content:   strings.Join(allResults, "\n\n"),
	}

	cachePath := filepath.Join(
		constants.ProjectsDir,
		data.ProjectID,
		constants.CacheFile,
	)
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return types.SummaryData{}, fmt.Errorf("failed to create cache dir: %w", err)
	}

	var existing types.CommitSummary

	// Read existing file if present
	if fileData, err := os.ReadFile(cachePath); err == nil {
		if err := json.Unmarshal(fileData, &existing); err != nil {
			return types.SummaryData{}, fmt.Errorf("invalid cache.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return types.SummaryData{}, fmt.Errorf("failed to read cache.json: %w", err)
	}

	// Set ProjectID if first write
	if existing.ProjectID == "" {
		existing.ProjectID = data.ProjectID
	}

	// Merge logic
	updated := false
	for i, item := range existing.Data {
		if item.CommitID == summaryData.CommitID {
			existing.Data[i] = summaryData
			updated = true
			break
		}
	}

	if !updated {
		existing.Data = append(existing.Data, summaryData)
	}

	// Atomic write
	tmp := cachePath + ".tmp"

	body, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return types.SummaryData{}, fmt.Errorf("failed to marshal cache.json: %w", err)
	}

	if err := os.WriteFile(tmp, body, 0644); err != nil {
		return types.SummaryData{}, fmt.Errorf("failed to write temp cache.json: %w", err)
	}

	if err := os.Rename(tmp, cachePath); err != nil {
		return types.SummaryData{}, fmt.Errorf("failed to replace cache.json: %w", err)
	}

	return summaryData, nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func diffString(a, b string) string {
	la := countLines(a)
	lb := countLines(b)
	if la > lb {
		return fmt.Sprintf("%d", la-lb)
	}
	return fmt.Sprintf("%d", lb-la)
}

func sortJobsBySizeAsc(jobs []types.Job) {
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Size < jobs[j].Size
	})
}

func countJobsBySize(jobs []types.Job, threshold int, small bool) int {
	cnt := 0
	for _, j := range jobs {
		if small && j.Size <= threshold {
			cnt++
		}
		if !small && j.Size > threshold {
			cnt++
		}
	}
	return cnt
}

func getAIProvider(name, projectDir string) (types.AIProvider, error) {
	// Check projectDir exists and is a directory
	info, err := os.Stat(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return types.AIProvider{}, fmt.Errorf("project directory does not exist: %s", projectDir)
		}
		return types.AIProvider{}, err
	}
	if !info.IsDir() {
		return types.AIProvider{}, fmt.Errorf("path is not a directory: %s", projectDir)
	}

	// Read info.txt
	data, err := os.ReadFile(filepath.Join(projectDir, "info.txt"))
	if err != nil {
		return types.AIProvider{}, fmt.Errorf("failed to read info.txt: %w", err)
	}

	// Unmarshal JSON
	var projectInfo types.Config
	if err := json.Unmarshal(data, &projectInfo); err != nil {
		return types.AIProvider{}, fmt.Errorf("invalid JSON in info.txt: %w", err)
	}

	// Lookup provider
	providers := projectInfo.AIProviders.AsMap()

	key, ok := providers[strings.ToLower(name)]
	if !ok || strings.TrimSpace(key) == "" {
		return types.AIProvider{}, fmt.Errorf("AI provider %q not configured", name)
	}

	return types.AIProvider{
		Name: name,
		Key:  key,
	}, nil
}
