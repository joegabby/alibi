package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joegabby/alibi/cli/internal/cli/helpers"
	"github.com/joegabby/alibi/cli/internal/cli/types"
	"github.com/spf13/cobra"
)

var (
	trackEvent string
	trackFrom  string
	trackTo    string
)

func init() {
	// register flags for the command (so hooks can call: alibi track --event=push --from=... --to=...)
	track.Flags().StringVar(&trackEvent, "event", "commit", "event type: commit|push")
	track.Flags().StringVar(&trackFrom, "from", "", "SHA or ref (for push)")
	track.Flags().StringVar(&trackTo, "to", "", "SHA or ref (for push)")
	rootCmd.AddCommand(track)
}

var track = &cobra.Command{
	Use:   "track",
	Short: "Tracks changes in the repo and records them in the global projects folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		commits, changes, timestamp, err := getChanges(trackEvent, trackFrom, trackTo)
		if err != nil {
			return err
		}

		activity := buildActivity(trackEvent, commits, changes, timestamp)

		// ✅ Create file name based on date
		fileName := time.Now().Format("2006-01-02") + ".json"

		fp := filepath.Join(cachedConfig.Project.Data, fileName)
		// ✅ Append activity data
		if err := appendToFile(fp, activity); err != nil {
			return err
		}

		data, err := json.Marshal(cachedConfig)
		if err != nil {
			return fmt.Errorf("error serializing config: %v", err)
		}

		ProjectInfo := filepath.Join(cachedConfig.Project.Data, "info.txt")
		if err := writeToFile(ProjectInfo, string(data)); err != nil {
			return err
		}

		fmt.Printf("✅ Tracked changes stored at %s\n", fp)
		return nil
	},
}

func shouldTrackFile(filePath string) bool {
	// Load config once
	if cachedConfig == nil {
		cfg, err := loadConfig()
		if err != nil {
			fmt.Println(err)
			return false
		}
		cachedConfig = cfg
	}

	if isHidden(filePath) && !isExplicitlyIncluded(filePath) {
		return false
	}

	// Check exclude rules first (exclude has higher priority)
	for _, pattern := range cachedConfig.Tracking.Exclude {
		if matchPattern(pattern, filePath) {
			return false
		}
	}

	// Check include rules
	for _, pattern := range cachedConfig.Tracking.Include {
		if matchPattern(pattern, filePath) {
			return true
		}
	}

	// Default: don't track
	return false
}

// Match patterns: *.js, src/**, folder, file.go
func matchPattern(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)

	// If pattern is a folder, exclude everything inside
	if !strings.Contains(pattern, ".") && !strings.Contains(pattern, "*") {
		if strings.HasPrefix(path, pattern) {
			return true
		}
	}

	// Recursive wildcard folder support e.g. src/**
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(path, base) {
			return true
		}
	}

	// File name or extension pattern
	ok, _ := filepath.Match(pattern, filepath.Base(path))
	if ok {
		return true
	}

	// Full path match
	ok, _ = filepath.Match(pattern, path)
	return ok
}

// Detect hidden files (.env, .git/*, etc)
func isHidden(path string) bool {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	return false
}

// Check if hidden file is explicitly included
func isExplicitlyIncluded(path string) bool {
	for _, pattern := range cachedConfig.Tracking.Include {
		if matchPattern(pattern, path) {
			return true
		}
	}
	return false
}

/*
	----------------------------
	  getChanges: gather commits + file changes
	  supports:
	   - event == "commit" : inspects last commit (HEAD)
	   - event == "push"   : requires --from and --to (refs/SHAs) to diff; fallback to commit if missing
	  returns: commits[], changes, timestamp (ISO)

----------------------------
*/
func shouldTrackCommit(commitHash string) bool {
	if cachedConfig == nil {
		cfg, err := loadConfig()
		if err != nil {
			fmt.Println(err)
			return false
		}
		cachedConfig = cfg
	}

	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	binDir := filepath.Dir(execPath)
	baseDir := filepath.Join(binDir, "..")

	projectsDir := filepath.Join(baseDir, "projects")
	projectID := cachedConfig.Project.Name

	projectData, err := loadProject(projectsDir, projectID)
	if err != nil {
		return false
	}
	for _, data := range projectData.Data.Activities {
		for _, commits := range data.Commits {
			if commits.ID == commitHash {
				fmt.Printf("⏩ Commit %s already tracked in project %s\n", commitHash, projectID)
				return false
			}
		}
	}
	return true
}

func getCommitChanges() ([]types.CommitInfo, []types.Changes, string, error) {
	commitHash, err := helpers.RunGit("rev-parse", "HEAD")
	if err != nil {
		return nil, nil, "", fmt.Errorf("unable to find HEAD: %w", err)
	}
	commitHash = strings.TrimSpace(commitHash)

	if !shouldTrackCommit(commitHash) {
		return nil, nil, "", fmt.Errorf("⏩ Commit %s already tracked in this project", commitHash)
	}

	msg, _ := helpers.RunGit("log", "-1", "--pretty=%B")
	msg = strings.TrimSpace(msg)

	ts, _ := helpers.RunGit("show", "-s", "--format=%cI", commitHash)
	ts = strings.TrimSpace(ts)
	if ts == "" {
		ts = time.Now().Format(time.RFC3339)
	}

	nameStatus, err := helpers.RunGit("show", "--name-status", "--pretty=", commitHash)
	if err != nil {
		return nil, nil, "", fmt.Errorf("git show failed: %w", err)
	}

	var changes []types.Changes
	scanner := bufio.NewScanner(strings.NewReader(nameStatus))

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		path := parts[len(parts)-1] // works for A/M/D or last arg in R100 old new

		if !shouldTrackFile(path) {
			fmt.Printf("⏩ Skipping %s (not included)\n", path)
			continue
		}
		fileChange := types.Changes{Path: path}
		switch {
		// 🟢 FILE ADDED
		case status == "A":
			fileChange.Type = "Added"
			newContent, _ := getFileContentAt(commitHash, path)
			fs := detectModified("", newContent, path)

			for _, fn := range fs.Added {
				lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
				fileChange.Functions = append(fileChange.Functions, types.FileChange{
					Name:       fn,
					Action:     "Added",
					LineCounts: lines,
				})
			}

		// 🟡 FILE MODIFIED
		case status == "M":
			fileChange.Type = "Modified"
			oldContent, _ := getFileContentAt(commitHash+"^", path)
			newContent, _ := getFileContentAt(commitHash, path)
			fs := detectModified(oldContent, newContent, path)

			for _, fn := range fs.Added {
				lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
				fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Added", LineCounts: lines})
			}
			for _, fn := range fs.Modified {
				lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
				fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Modified", LineCounts: lines})
			}
			for _, fn := range fs.Removed {
				fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Removed"})
			}
			for _, fn := range fs.Muted {
				fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Muted"})
			}

		// 🔴 FILE DELETED
		case status == "D":
			fileChange.Type = "Deleted"
			oldContent, _ := getFileContentAt(commitHash+"^", path)
			fs := detectModified(oldContent, "", path)

			for _, fn := range fs.Removed {
				fileChange.Functions = append(fileChange.Functions, types.FileChange{
					Name:   fn,
					Action: "Removed",
				})
			}

		// 🟣 FILE RENAMED
		default:
			if strings.HasPrefix(status, "R") && len(parts) >= 3 {
				oldPath := parts[1]
				newPath := parts[2]

				fileChange.Type = "Renamed"
				fileChange.Path = fmt.Sprintf("%s -- %s", oldPath, newPath)

				oldContent, _ := getFileContentAt(commitHash+"^", oldPath)
				newContent, _ := getFileContentAt(commitHash, newPath)
				fs := detectModified(oldContent, newContent, newPath)

				for _, fn := range fs.Muted {
					lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
					fileChange.Functions = append(fileChange.Functions, types.FileChange{
						Name:       fn,
						Action:     "Renamed",
						LineCounts: lines,
					})
				}
				for _, fn := range fs.Added {
					lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
					fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Added", LineCounts: lines})
				}
				for _, fn := range fs.Modified {
					lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
					fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Modified", LineCounts: lines})
				}
				for _, fn := range fs.Removed {
					lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
					fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Removed", LineCounts: lines})
				}
				for _, fn := range fs.Muted {
					lines := helpers.ExtractFunctionDiffUsingTreeSitter(commitHash, path, fn)
					fileChange.Functions = append(fileChange.Functions, types.FileChange{Name: fn, Action: "Muted", LineCounts: lines})
				}
			}
		}

		if len(fileChange.Functions) > 0 || fileChange.Type != "" {
			changes = append(changes, fileChange)
		}
	}

	return []types.CommitInfo{{ID: commitHash, Message: msg, Timestamp: ts}}, changes, ts, nil
}

func getChanges(event, from, to string) ([]types.CommitInfo, []types.Changes, string, error) {
	if event == "commit" {
		return getCommitChanges()
	}

	if event == "push" {
		commits := []types.CommitInfo{}
		changes := []types.Changes{}

		if cachedConfig == nil {
			cfg, err := loadConfig()
			if err != nil {
				return nil, []types.Changes{}, "", fmt.Errorf("unable to load config: %w", err)
			}
			cachedConfig = cfg
		}
		// case 1: infer commits from tracking branch if from/to not set
		if from == "" || to == "" {

			// ⭐ Check if this is the FIRST push
			hasRemote, _ := helpers.RunGit(
				"ls-remote",
				"--heads",
				cachedConfig.Tracking.Remote,
				cachedConfig.Tracking.Branch,
			)

			if strings.TrimSpace(hasRemote) == "" {
				// 👉 This is the FIRST PUSH — no interval exists

				// Get only HEAD commit
				headCommit, err := helpers.RunGit(
					"log", "-1", "--pretty=format:%H|%s|%cI",
				)
				if err != nil {
					return nil, nil, "", fmt.Errorf("unable to read HEAD commit: %w", err)
				}

				parts := strings.SplitN(headCommit, "|", 3)
				ci := types.CommitInfo{
					ID:        parts[0],
					Message:   parts[1],
					Timestamp: parts[2],
				}

				return []types.CommitInfo{ci}, changes, ci.Timestamp, nil
			}

			// Normal behavior (not first push)
			commitIntervals, err := helpers.RunGit(
				"log",
				fmt.Sprintf("%s/%s..HEAD", cachedConfig.Tracking.Remote, cachedConfig.Tracking.Branch),
				"--pretty=format:%H|%s|%cI",
			)
			if err != nil {
				return nil, []types.Changes{}, "", fmt.Errorf("unable to find commits in-between pushes: %w", err)
			}

			groupedCommits := strings.Split(strings.TrimSpace(commitIntervals), "\n")
			for _, commit := range groupedCommits {
				if strings.TrimSpace(commit) == "" {
					continue
				}
				parts := strings.SplitN(commit, "|", 3)
				ci := types.CommitInfo{ID: parts[0]}
				if len(parts) > 1 {
					ci.Message = parts[1]
					ci.Timestamp = parts[2]
				}
				commits = append(commits, ci)
			}

			return commits, changes, time.Now().UTC().Format(time.RFC3339), nil
		}

		return commits, changes, time.Now().UTC().Format(time.RFC3339), nil
	}

	return nil, []types.Changes{}, "", nil
}

// isNullRef reports whether ref is Git's all-zero object id, which hooks pass
// to mean "no such commit" — for example the far side of a brand-new branch
// push. Git writes 40 zeros for SHA-1 repos and 64 for SHA-256, so this checks
// that every character is a zero rather than counting them.
func isNullRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	return ref != "" && strings.Trim(ref, "0") == ""
}

// getFileContentAt(ref, path): returns file content at that ref:path
// if ref is empty or ref is 0000... returns "", nil
func getFileContentAt(ref, path string) (string, error) {
	if ref == "" || isNullRef(ref) {
		return "", nil
	}
	out, err := helpers.RunGit("show", fmt.Sprintf("%s:%s", ref, path))
	if err != nil {
		// file may not exist at that ref
		return "", nil
	}
	return out, nil
}

/*
	----------------------------
	  detectModified: compare old and new file content and return FunctionSet
	  - uses Go AST for .go files
	  - falls back to regex heuristics otherwise

----------------------------
*/

func detectModified(oldContent, newContent, path string) types.FunctionSet {
	ext := strings.ToLower(filepath.Ext(path))

	oldFuncs := map[string]string{}
	newFuncs := map[string]string{}

	oldFuncs = helpers.ExtractFunctions(ext, oldContent)
	newFuncs = helpers.ExtractFunctions(ext, newContent)

	fs := types.FunctionSet{}
	// detect added & removed
	for name := range newFuncs {
		if _, ok := oldFuncs[name]; !ok {
			fs.Added = append(fs.Added, name)
		}
	}
	for name := range oldFuncs {
		if _, ok := newFuncs[name]; !ok {
			// candidate removed; could be muted (commented out)
			if isFunctionCommented(name, newContent) {
				fs.Muted = append(fs.Muted, name)
			} else {
				fs.Removed = append(fs.Removed, name)
			}
		}
	}
	// detect modified: present in both but body differs
	for name, oldBody := range oldFuncs {
		if newBody, ok := newFuncs[name]; ok {
			if strings.TrimSpace(oldBody) != strings.TrimSpace(newBody) {
				fs.Modified = append(fs.Modified, name)
			}
		}
	}
	return fs
}

// isFunctionCommented reports whether name is absent from the parsed functions
// but still present in newContent inside a comment — i.e. it was commented out
// rather than deleted.
//
// It strips the file down to its comments first, then looks for a declaration
// of name in what remains. The previous version treated the mere presence of a
// "/*" anywhere in the file as proof, which marked every removal in any file
// containing a block comment as Muted.
func isFunctionCommented(name, newContent string) bool {
	if name == "" || strings.TrimSpace(newContent) == "" {
		return false
	}

	comments := commentedText(newContent)
	if strings.TrimSpace(comments) == "" {
		return false
	}

	return declarationPattern(name).MatchString(comments)
}

// declarationPattern matches a function declaration for name across the
// languages Alibi parses: keyword forms (func/def/function/fn/sub), assigned
// forms (name = function… / name := (…) =>), and bare method signatures.
func declarationPattern(name string) *regexp.Regexp {
	q := regexp.QuoteMeta(name)
	return regexp.MustCompile(
		`\b(?:func|def|function|fn|sub)\b[^\n]*\b` + q + `\b` +
			`|\b` + q + `\b\s*[:=]+\s*(?:async\s+)?(?:function\b|\()` +
			`|\b` + q + `\b\s*\([^)\n]*\)\s*[:{]`)
}

// commentedText returns only the parts of content that sit inside comments,
// recognising // and # line comments and /* */ blocks. That covers every
// language in treeSitterParsers. "--" is deliberately not a marker: no parsed
// language uses it, and it would misread the decrement operator in `i--`.
//
// It deliberately does not track string literals, so a "http://…" inside a
// string can leak through as comment text. That is harmless here: callers only
// search the result for function declarations, which a URL will not match.
func commentedText(content string) string {
	var out strings.Builder

	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		rest := line

		for rest != "" {
			if inBlock {
				end := strings.Index(rest, "*/")
				if end < 0 {
					out.WriteString(rest)
					break
				}
				out.WriteString(rest[:end])
				rest = rest[end+2:]
				inBlock = false
				continue
			}

			blockAt := strings.Index(rest, "/*")
			lineAt := earliestLineComment(rest)

			switch {
			case blockAt >= 0 && (lineAt < 0 || blockAt < lineAt):
				rest = rest[blockAt+2:]
				inBlock = true
			case lineAt >= 0:
				out.WriteString(rest[lineAt:])
				rest = ""
			default:
				rest = ""
			}
		}

		out.WriteByte('\n')
	}

	return out.String()
}

// earliestLineComment returns the index of the first line-comment marker in s,
// or -1 if there is none.
func earliestLineComment(s string) int {
	found := -1
	for _, marker := range []string{"//", "#"} {
		if i := strings.Index(s, marker); i >= 0 && (found < 0 || i < found) {
			found = i
		}
	}
	return found
}

// buildActivity: compose the activity object
func buildActivity(event string, commits []types.CommitInfo, changes []types.Changes, ts string) types.Activity {
	id := fmt.Sprintf("%x", time.Now().UnixNano())
	return types.Activity{
		ID:          id,
		Type:        event,
		Timestamp:   ts,
		Commits:     commits,
		FileChanges: changes,
	}
}

// appendToFile: append activity to daily file
func appendToFile(path string, activity types.Activity) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var tracker types.TrackerFile
	if _, err := os.Stat(path); err == nil {
		// file exists -> read
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &tracker); err != nil {
				// if corrupted, create new
				tracker = types.TrackerFile{Date: time.Now().Format("2006-01-02")}
			}
		}
	} else {
		tracker = types.TrackerFile{Date: time.Now().Format("2006-01-02")}
	}

	tracker.Activities = append(tracker.Activities, activity)
	out, err := json.MarshalIndent(tracker, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	return nil
}

// Helper function to write data to a file
func writeToFile(filePath, data string) error {
	// Ensure the directory exists (create it if necessary)
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("error creating directory: %v", err)
		}
	}

	// Create or open the file for writing (create if it doesn't exist, truncate if it does)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close() // Close the file when we're done

	// Write the provided data to the file
	_, err = file.WriteString(data)
	if err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}
