// Package output validates the generator-owned run directory structure.
package output

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var allowedRunFiles = []*regexp.Regexp{
	regexp.MustCompile(`^runs/[^/]+/manifest\.json$`),
	regexp.MustCompile(`^runs/[^/]+/contact-sheets/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/contact-sheets/candidates/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/contact-sheets/intermediates/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/(prompt\.md|qa\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/review/(contact-sheet\.png|animation\.gif)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/(raw-candidate\.png|mask\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/identity/(prompt\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/identity/attempts/[^/]+/raw-candidate\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/directions/[^/]+/(prompt\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/directions/[^/]+/attempts/[^/]+/raw-candidate\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/identity/board\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/variants/[^/]+/anchor/(prompt\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/variants/[^/]+/anchor/attempts/[^/]+/mask\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/variants/[^/]+/anchor/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/pose-board\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/motion-study/(prompt\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/motion-study/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/direction-seeds/(source-board\.png|prompt\.md|qa\.md|normalized\.png|edit-source\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/direction-seeds/seeds/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/direction-seeds/review/(candidates\.png|contact-sheet\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/direction-seeds/attempts/[^/]+/mask\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/direction-seeds/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/row/(pose-board\.png|edit-source\.png|prompt\.md|qa\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/row/pose-guides/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/row/attempts/[^/]+/mask\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/row/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/[^/]+/row/review/(candidates\.png|contact-sheet\.png|animation\.gif)$`),
}

// Validate rejects files outside the versioned run layouts. The legacy tree is
// intentionally opaque because it stores drafts migrated from the pre-CLI flow.
func Validate(root string) error {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == "legacy" {
				return filepath.SkipDir
			}
			return nil
		}
		for _, pattern := range allowedRunFiles {
			if pattern.MatchString(relative) {
				return nil
			}
		}
		return fmt.Errorf("unexpected managed output file %q", strings.TrimPrefix(relative, "./"))
	})
}
