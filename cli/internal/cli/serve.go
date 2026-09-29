package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	// "strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/joegabby/alibi/web"
	"github.com/joegabby/alibi/cli/internal/cli/constants"
	"github.com/joegabby/alibi/cli/internal/cli/helpers"
	"github.com/joegabby/alibi/cli/internal/cli/pdf"
	"github.com/joegabby/alibi/cli/internal/cli/types"
	"github.com/spf13/cobra"
)

// --------------------- Types ---------------------

// --------------------- Command Setup ---------------------

var port uint

func init() {
	serve.Flags().UintVar(&port, "port", 2025, "port number")
	rootCmd.AddCommand(serve)
}

var serve = &cobra.Command{
	Use:   "serve",
	Short: "Serve the tracked progress dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		actualPort, err := ServeChanges(port)
		if err != nil {
			return err
		}
		fmt.Printf("\033[32mDashboard ---------> http://localhost:%d\033[0m\n", actualPort)
		fmt.Println("Press CTRL+C to stop the server.")
		select {}
	},
}

// --------------------- Core Server ---------------------

func ServeChanges(defaultPort uint) (uint, error) {
	execPath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %v", err)
	}

	binDir := filepath.Dir(execPath)
	baseDir := filepath.Join(binDir, "..")

	projectsDir := filepath.Join(baseDir, "projects")
	dashboardDir, err := fs.Sub(web.Dashboard, "dashboard")
	if err != nil {
		return 0, fmt.Errorf("failed to access dashboard files: %v", err)
	}

	// dashboardDir := web.Dashboard.Open("dashboard/index.html")

	// 🔒 Initialize project cache with RWMutex
	cache := make(map[string]types.ProjectData)
	var cacheMu sync.RWMutex

	// 👀 Start file watcher to invalidate cache when files change
	go watchProjectsDir(projectsDir, func(projectID string) {
		cacheMu.Lock()
		delete(cache, projectID)
		cacheMu.Unlock()
		// fmt.Printf("🧹 Cache invalidated for project: %s\n", projectID)
	})

	http.HandleFunc("/api/generate-pdf", func(w http.ResponseWriter, r *http.Request) {

		var incoming types.DownloadPDF

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", 500)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// 1. Decode incoming JSON
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			helpers.SendSSE(w, flusher, "error", err.Error())
			return
		}

		// ProjectID becomes part of the path the report is written to, so it has
		// to be checked before it reaches filepath.Join.
		if !validProjectID(incoming.ProjectID) {
			helpers.SendSSE(w, flusher, "error", "invalid project id")
			return
		}

		ctx := r.Context()

		emptyCount := 0

		if incoming.UseAI {
			for _, activity := range incoming.Activities {
				if len(activity.Commits) == 0 {
					continue
				}

				if strings.ToLower(activity.Type) == "commit" &&
					strings.TrimSpace(activity.Commits[0].Summary.Content) == "" {
					emptyCount++
				}
			}
			helpers.SendSSE(w, flusher, "summary", fmt.Sprintf("Found %d empty summaries", emptyCount))

		}

		if incoming.UseAI {
			for _, activity := range incoming.Activities {
				// Guard against empty commits
				if len(activity.Commits) == 0 {
					continue
				}

				// Check for empty summary
				if strings.ToLower(activity.Type) == "commit" && strings.TrimSpace(activity.Commits[0].Summary.Content) == "" {
					allResults := make([]string, 0, len(activity.Commits))

					// Build path list
					pathList := make([]string, 0, len(activity.FileChanges))
					for _, change := range activity.FileChanges {
						pathList = append(pathList, change.Path)
					}

					data := types.CommitData{
						ProjectID: incoming.ProjectID,
						Hash:      activity.Commits[0].ID,
						Message:   activity.Commits[0].Message,
						Paths:     pathList,
						Provider:  incoming.Provider,
						Model:     incoming.Model,
						Source:    incoming.Source,
						Dir:       incoming.Directory,
					}

					// ✅ Always use a valid context

					events := helpers.ProcessSummaries(ctx, data)

					for evt := range events {
						switch evt.Type {

						case "summary":
							result, ok := evt.Data.(types.SummaryResult)
							if !ok {
								log.Println("invalid summary payload")
								continue
							}
							allResults = append(allResults, result.HTML)
							helpers.SendSSE(w, flusher, "summary", "")

						case "saved":
							// The event carries provider/model/timestamp but
							// deliberately NOT the content — that is stripped
							// upstream to keep the SSE frame small. Assigning it
							// wholesale therefore blanks the summary and the
							// report silently falls back to the raw change list.
							// Take the metadata from the record and the text from
							// the chunks collected above.
							summary := types.SummaryData{
								Provider: incoming.Provider,
								Model:    incoming.Model,
							}
							if saved, ok := evt.Data.(types.SummaryData); ok {
								summary = saved
							}
							summary.Content = strings.Join(allResults, "")
							activity.Commits[0].Summary = summary

							helpers.SendSSE(w, flusher, "saved", "Saving summaries")
						case "error":
							helpers.SendSSE(w, flusher, "error", evt.Data)
							log.Println("summary error:", evt.Data)
						}
					}
				}

			}
		}
		html := pdf.RenderReport(incoming.Activities, incoming.ProjectID, incoming.StartDate, incoming.EndDate)

		var (
			fileName string
			contents []byte
		)

		pdfBytes, err := pdf.GeneratePDF(html)
		switch {
		case errors.Is(err, pdf.ErrChromiumNotFound):
			// No browser to print with, so hand over the report itself. It is
			// the same document with the same styling, and it opens and prints
			// to PDF from any browser — which beats failing the export outright.
			fileName = pdf.ReportFileName(incoming.ProjectID, incoming.StartDate, incoming.EndDate, "html")
			contents = []byte(html)
			helpers.SendSSE(w, flusher, "notice",
				"No browser found to print a PDF — saved an HTML report you can open and print instead.")
			log.Println("PDF export fell back to HTML:", err)

		case err != nil:
			helpers.SendSSE(w, flusher, "error", err.Error())
			log.Println("PDF generation error:", err)
			return

		default:
			fileName = pdf.ReportFileName(incoming.ProjectID, incoming.StartDate, incoming.EndDate, "pdf")
			contents = pdfBytes
		}

		if err := os.WriteFile(filepath.Join(projectsDir, incoming.ProjectID, fileName), contents, 0644); err != nil {
			helpers.SendSSE(w, flusher, "error", err.Error())
			return
		}
		helpers.SendSSE(w, flusher, "pdf-generated", fileName)
		fmt.Printf("\033[34mNew report file downloaded: %s\033[0m\n", fileName)
	})

	http.HandleFunc("/api/download-pdf", func(w http.ResponseWriter, r *http.Request) {
		projectID := r.URL.Query().Get("projectId")
		docID := r.URL.Query().Get("docId")

		if !validProjectID(projectID) {
			http.Error(w, "Invalid or missing project id", http.StatusBadRequest)
			return
		}
		if !validDocID(docID) {
			http.Error(w, "Invalid or missing document id", http.StatusBadRequest)
			return
		}

		path := filepath.Join(projectsDir, projectID, docID)

		// Reports are saved as PDF, or as HTML when no browser was available to
		// print one.
		contentType := "application/pdf"
		if strings.HasSuffix(docID, ".html") {
			contentType = "text/html; charset=utf-8"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", docID))
		http.ServeFile(w, r, path)
	})

	// 🧩 List the models a provider can serve, with pricing where published.
	// The provider's API key is read server-side and never sent to the browser.
	http.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		providerName := r.URL.Query().Get("provider")
		projectID := r.URL.Query().Get("project")

		if providerName == "" {
			http.Error(w, "Missing provider", http.StatusBadRequest)
			return
		}
		if !validProjectID(projectID) {
			http.Error(w, "Invalid or missing project id", http.StatusBadRequest)
			return
		}

		projectDir := filepath.Join(projectsDir, projectID)
		models, err := helpers.ListModelsForProject(r.Context(), providerName, projectDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		respondJSON(w, models)
	})

	// 📁 List all projects
	http.HandleFunc("/api/projects", listProjectsHandler(projectsDir))

	// get AI summary TEST
	http.HandleFunc("/api/ai-summary", helpers.SummaryStreamHandler)

	// 📂 Get a specific project
	http.HandleFunc("/api/project", func(w http.ResponseWriter, r *http.Request) {
		projectID := r.URL.Query().Get("id")
		if !validProjectID(projectID) {
			http.Error(w, "Invalid or missing project id", http.StatusBadRequest)
			return
		}

		// Serve from cache if available
		cacheMu.RLock()
		cached, ok := cache[projectID]
		cacheMu.RUnlock()
		if ok {
			respondJSON(w, cached)
			return
		}

		project, err := loadProject(projectsDir, projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Save in cache
		cacheMu.Lock()
		cache[projectID] = project
		cacheMu.Unlock()

		respondJSON(w, project)
	})
	http.HandleFunc("/api/commit", func(w http.ResponseWriter, r *http.Request) {
		commitHash := r.URL.Query().Get("id")
		projectID := r.URL.Query().Get("project")

		if commitHash == "" || projectID == "" {
			http.Error(w, "Missing commit id or project id", http.StatusBadRequest)
			return
		}

		if !validProjectID(projectID) {
			http.Error(w, "Invalid project id", http.StatusBadRequest)
			return
		}

		project, err := loadProject(projectsDir, projectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		for _, activity := range project.Data.Activities {
			if activity.Type == "commit" {
				for _, commit := range activity.Commits {
					if commit.ID == commitHash {
						respondJSON(w, activity) // return just the commit
						return
					}
				}
			}
		}

		http.Error(w, "Commit not found", http.StatusNotFound)
	})

	// Serve static dashboard
	http.Handle("/", http.FileServer(http.FS(dashboardDir)))

	// 🧠 Start server on available port
	listener, actualPort, err := getAvailablePort(defaultPort)
	if err != nil {
		return 0, err
	}

	go func() {
		if err := http.Serve(listener, nil); err != nil {
			fmt.Println("Server error:", err)
		}
	}()

	return actualPort, nil
}

// --------------------- Handlers ---------------------

func listProjectsHandler(projectsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dirs, err := os.ReadDir(projectsDir)
		if err != nil {
			http.Error(w, "Failed to read projects directory", http.StatusInternalServerError)
			return
		}

		var summaries []types.ProjectSummary
		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}

			projectPath := filepath.Join(projectsDir, dir.Name())
			lastUpdated := getLastUpdated(projectPath)

			summaries = append(summaries, types.ProjectSummary{
				ID:          dir.Name(),
				Name:        dir.Name(),
				LastUpdated: lastUpdated,
			})
		}

		// ✅ Sort by LastUpdated (most recent first)
		sort.Slice(summaries, func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, summaries[i].LastUpdated)
			tj, _ := time.Parse(time.RFC3339, summaries[j].LastUpdated)
			return ti.After(tj)
		})

		respondJSON(w, summaries)
	}
}

// --------------------- Helpers ---------------------

func loadProject(projectsDir, projectID string) (types.ProjectData, error) {
	projectPath := filepath.Join(projectsDir, projectID)
	files, err := os.ReadDir(projectPath)
	if err != nil {
		return types.ProjectData{}, fmt.Errorf("project not found")
	}

	var merged types.TrackerFile
	var lastUpdated string

	infoFilePath := filepath.Join(projectPath, "info.txt")
	infoFileContent, err := os.ReadFile(filepath.Join(infoFilePath))

	if err != nil {
		return types.ProjectData{}, fmt.Errorf("Error reading info.txt:")
	}
	var info types.Config
	if err := json.Unmarshal(infoFileContent, &info); err != nil {
		return types.ProjectData{}, fmt.Errorf("Error unmarshalling info.txt content:%s", err)
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" || f.Name() == constants.CacheFile {
			continue
		}

		content, err := os.ReadFile(filepath.Join(projectPath, f.Name()))
		if err != nil {
			continue
		}

		var tf types.TrackerFile
		if err := json.Unmarshal(content, &tf); err != nil {
			continue
		}

		merged.Activities = append(merged.Activities, tf.Activities...)
	}

	sort.Slice(merged.Activities, func(i, j int) bool {
		return merged.Activities[i].Timestamp > merged.Activities[j].Timestamp
	})

	for _, act := range merged.Activities {
		if lastUpdated == "" || act.Timestamp > lastUpdated {
			lastUpdated = act.Timestamp
		}
	}

	injectURLs(&merged, info.Tracking.Repo, projectPath)

	return types.ProjectData{
		Name:        projectID,
		Repo:        info.Tracking.Repo,
		AIProviders: enabledProviders(info.AIProviders),
		Path:        projectPath,
		Source:      info.Project.Source,
		Data:        merged,
		LastUpdated: lastUpdated,
	}, nil
}

func enabledProviders(c types.ConfigAI) []string {
	providers := map[string]string{
		"OpenAI":      c.OpenAI,
		"DeepSeek":    c.DeepSeek,
		"OpenRouter":  c.OpenRouter,
		"HuggingFace": c.HuggingFace,
	}

	var enabled []string
	for name, key := range providers {
		if key != "" {
			enabled = append(enabled, name)
		}
	}

	return enabled
}

func getLastUpdated(projectPath string) string {
	files, err := os.ReadDir(projectPath)
	if err != nil {
		return ""
	}

	var lastUpdated string
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" || f.Name() == constants.CacheFile {
			continue
		}

		content, err := os.ReadFile(filepath.Join(projectPath, f.Name()))
		if err != nil {
			continue
		}

		var tf types.TrackerFile
		if err := json.Unmarshal(content, &tf); err != nil {
			continue
		}

		for _, act := range tf.Activities {
			if lastUpdated == "" || act.Timestamp > lastUpdated {
				lastUpdated = act.Timestamp
			}
		}
	}
	return lastUpdated
}

func injectURLs(tf *types.TrackerFile, repoURL string, projectPath string) {
	host, owner, repo, err := parseRepoURL(repoURL)
	if err != nil {
		fmt.Printf("Error parsing repo URL '%s': %v", repoURL, err)
		return
	}

	githubBase := fmt.Sprintf("https://%s/%s/%s", host, owner, repo)

	cachedFile := filepath.Join(projectPath, constants.CacheFile)
	cachedFileContent, err := os.ReadFile(filepath.Join(cachedFile))
	var cache types.CommitSummary
	json.Unmarshal(cachedFileContent, &cache)
	for ai, act := range tf.Activities {
		for ci, commit := range act.Commits {
			var summarized types.SummaryData
			for _, summary := range cache.Data {
				if summary.CommitID == commit.ID {
					summarized = summary
				}
			}

			if act.Type == "push" {
				tf.Activities[ai].Commits[ci].AlibiViewURL =
					fmt.Sprintf("/commit/%s", commit.ID)
				tf.Activities[ai].Commits[ci].GithubURL =
					fmt.Sprintf("%s/commit/%s", githubBase, commit.ID)
			} else if act.Type == "commit" {
				tf.Activities[ai].Commits[ci].GithubURL =
					fmt.Sprintf("%s/commit/%s", githubBase, commit.ID)
				tf.Activities[ai].Commits[ci].Summary = summarized
			}
		}
	}
}

// --------------------- Watcher ---------------------
func watchProjectsDir(projectsDir string, onChange func(projectID string)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Watcher error:", err)
		return
	}
	defer watcher.Close()

	filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d != nil && d.IsDir() {
			watcher.Add(path)
		}
		return nil
	})

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				projectID := filepath.Base(filepath.Dir(event.Name))
				onChange(projectID)
			}
		case err, ok := <-watcher.Errors:
			if ok {
				fmt.Println("Watcher error:", err)
			}
		}
	}
}

// --------------------- Utilities ---------------------
func validProjectID(id string) bool {
	return id != "" && !strings.Contains(id, "..") && !strings.ContainsAny(id, "/\\")
}

// docIDPattern matches the report names ReportFileName produces:
// <project>_<start>_<end>_<unix>.<pdf|html>. The character class excludes path
// separators and CR/LF, so a validated docID can neither escape the project
// directory nor be injected into the Content-Disposition header.
var docIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+\.(pdf|html)$`)

func validDocID(id string) bool {
	return !strings.Contains(id, "..") && docIDPattern.MatchString(id)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func getAvailablePort(defaultPort uint) (net.Listener, uint, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", defaultPort))
	if err == nil {
		return listener, defaultPort, nil
	}
	listener, err = net.Listen("tcp", ":0")
	if err != nil {
		return nil, 0, err
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	return listener, uint(actualPort), nil
}
