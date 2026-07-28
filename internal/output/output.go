// Package output validates the generator-owned run directory structure.
package output

import (
	"encoding/json"
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
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/(prompt\.md|qa\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/review/(contact-sheet\.png|animation\.gif)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/(current-directional-references\.png|layout-source\.png|prompt\.md|qa\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/provider/layout-source\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/provider/direction-references/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/recovered/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/review/(candidates|ownership|recovered-poses)\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/(layout-source\.png|master-comparison-guide\.png|prompt\.md|qa\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/provider/layout-source\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/recovered/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/review/(candidates|ownership|recovered-poses)\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/(palette\.json|qa\.md)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/(complete-unit\.png|master-to-animation\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/master-directions/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/gifs/[^/]+\.gif$`),
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
			if unsupported, err := unsupportedRun(path, relative); err != nil {
				return err
			} else if unsupported {
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

func unsupportedRun(path, relative string) (bool, error) {
	parts := strings.Split(relative, "/")
	if len(parts) != 2 || parts[0] != "runs" {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return false, fmt.Errorf("decode run manifest %q: %w", filepath.Join(relative, "manifest.json"), err)
	}
	return header.Version != 9, nil
}
