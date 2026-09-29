package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"github.com/joegabby/alibi/cli/internal/cli/types"
)

func init() {
	rootCmd.AddCommand(doctor)
}


func loadConfig() (*types.Config, error) {
	data, err := os.ReadFile("alibi.yml")
	if err != nil {
		return nil, err
	}

	var cfg types.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// parseRepoURL handles only:
//  1. github.com/owner/repo
//  2. https://github.com/owner/repo.git
func parseRepoURL(repoURL string) (string, string, string, error) {
	repoURL = strings.TrimSpace(repoURL)

	// Add protocol if missing
	if !strings.Contains(repoURL, "://") {
		repoURL = "https://" + repoURL
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid repo URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid repo path: %s", repoURL)
	}

	owner := parts[len(parts)-2]
	repo := strings.TrimSuffix(parts[len(parts)-1], ".git")

	host := u.Hostname() // Full host, e.g., github.com

	return host, owner, repo, nil
}

var doctor = &cobra.Command{
	Use:   "doctor",
	Short: "Test the GitHub connection using settings in the config file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		_, owner, repo, err := parseRepoURL(cfg.Tracking.Repo)
		if err != nil {
			return fmt.Errorf("failed to extract owner/repo: %w", err)
		}

		fmt.Printf("🔍 Checking repo: owner=%s repo=%s\n", owner, repo)

		// --- Verify local git remote
		cmdGit := exec.Command("git", "remote", "get-url", cfg.Tracking.Remote)
		out, err := cmdGit.Output()
		if err != nil {
			return fmt.Errorf("failed to get local git remote: %w", err)
		}
		localRemote := strings.TrimSpace(string(out))

		// Normalize URLs for comparison
		norm := func(s string) string {
			s = strings.TrimPrefix(s, "https://")
			s = strings.TrimSuffix(s, ".git")
			return strings.TrimSpace(s)
		}

		if !strings.Contains(norm(localRemote), fmt.Sprintf("%s/%s", owner, repo)) {
			return fmt.Errorf("mismatch: config repo=%s but local origin=%s", cfg.Tracking.Repo, localRemote)
		}

		fmt.Println("✅ Config repo matches local remote")

		// check project directory
		projectDir := cfg.Project.Data
		info, err := os.Stat(projectDir)
		fmt.Println(projectDir)
		if os.IsNotExist(err) {
			return fmt.Errorf("project directory not found: %s", projectDir)
		}
		if err != nil {
			return fmt.Errorf("failed to access project directory: %v", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", projectDir)
		}

		fmt.Println("✅ project directory confirmed")

		return nil
	},
}
