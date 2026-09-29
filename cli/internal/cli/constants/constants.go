package constants

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	ExecPath     string
	BinDir       string
	BaseDir      string
	ProjectsDir  string
	DashboardDir string
	CacheFile    string
)

func init() {
	var err error

	ExecPath, err = os.Executable()
	if err != nil {
		panic(fmt.Errorf("failed to get executable path: %w", err))
	}

	BinDir = filepath.Dir(ExecPath)
	BaseDir = filepath.Clean(filepath.Join(BinDir, ".."))

	ProjectsDir = filepath.Join(BaseDir, "projects")
	DashboardDir = filepath.Join(BaseDir, "dashboard")
	
	CacheFile = "cache.json"

}
