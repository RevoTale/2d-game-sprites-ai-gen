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
	regexp.MustCompile(`^catalog/(catalog\.json|index\.html)$`),
	regexp.MustCompile(`^catalog/assets/[a-z0-9][a-z0-9-]*\.png$`),
	regexp.MustCompile(`^runs/[^/]+/manifest\.json$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/(prompt\.md|qa\.md|normalized\.png|palette\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/review/(contact-sheet\.png|animation\.gif|style-guide-96\.png|battlefield-preview-96\.png|logical-preview\.png|tiled-repeat-3x3\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/targets/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/(layout-source\.png|edit-mask\.png|prompt\.md|qa\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/provider/layout-source\.png$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/recovered/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/review/(candidates|ownership|recovered-poses|native-parts|logical-parts)\.png$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/review/runtime-overrides/[a-z0-9][a-z0-9-]*\.png$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/static-sets/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/(canonical-profile\.json|canonical-profile-overlay\.png|current-directional-references\.png|layout-source\.png|prompt\.md|qa\.md|normalized\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/provider/layout-source\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/provider/direction-references/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/recovered/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/review/(candidates|ownership|recovered-poses)\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/character-master/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/(layout-source\.png|master-comparison-guide\.png|prompt\.md|qa\.md|normalized\.png|scale-calibration\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/provider/layout-source\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/recovered/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/review/(candidates|ownership|recovered-poses)\.png$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/attempts/[^/]+/evidence\.json$`),
	regexp.MustCompile(`^runs/[^/]+/intermediates/[^/]+/animations/[^/]+/attempts/[^/]+/candidates/[^/]+/(raw-candidate\.png|normalized\.png|metrics\.json)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/(palette\.json|qa\.md)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/(complete-unit\.png|master-to-animation\.png|portrait-96\.png)$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/master-directions/[^/]+\.png$`),
	regexp.MustCompile(`^runs/[^/]+/units/[^/]+/review/gifs/[^/]+\.gif$`),
}

// Validate rejects files outside the current run layout.
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
