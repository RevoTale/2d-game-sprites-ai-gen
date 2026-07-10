package gitguard

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Check returns tracked or staged PNG files below generatedRoot.
func Check(generatedRoot string) ([]string, error) {
	if _, err := os.Stat(generatedRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	absGeneratedRoot, err := filepath.Abs(generatedRoot)
	if err != nil {
		return nil, err
	}
	repoRoot, err := gitOutput(generatedRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("find git root for %s: %w", generatedRoot, err)
	}
	repoRoot = strings.TrimSpace(repoRoot)
	relativeRoot, err := filepath.Rel(repoRoot, absGeneratedRoot)
	if err != nil {
		return nil, err
	}
	if relativeRoot == ".." || strings.HasPrefix(relativeRoot, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("%s is outside git root %s", generatedRoot, repoRoot)
	}

	tracked, err := listedPNGs(repoRoot, "ls-files", "--", relativeRoot)
	if err != nil {
		return nil, err
	}
	staged, err := listedPNGs(repoRoot, "diff", "--cached", "--name-only", "--", relativeRoot)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var offenders []string
	for _, path := range append(tracked, staged...) {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		offenders = append(offenders, path)
	}
	slices.Sort(offenders)
	return offenders, nil
}

func listedPNGs(repoRoot string, args ...string) ([]string, error) {
	out, err := gitOutput(repoRoot, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".png") {
			files = append(files, line)
		}
	}
	return files, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}
	message := strings.TrimSpace(stderr.String())
	if message == "" {
		message = err.Error()
	}
	return "", errors.New(message)
}
