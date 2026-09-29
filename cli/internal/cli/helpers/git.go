package helpers

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func RunGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

func RunGitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir // This makes git run in the desired directory

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git error in %s: %v\n%s", dir, err, string(output))
	}

	return string(output), nil
}
