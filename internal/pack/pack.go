// Package pack loads and validates THEME.md plus sprites.json sprite-pack definitions.
package pack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultOutputDir              = "output"
	DefaultStaticDeployTemplate   = "sprites/{target}.png"
	DefaultAnimatedDeployTemplate = "units/{object}__{animation}__{variant.direction}__{frame}.png"
)

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	placeholderPattern = regexp.MustCompile(`\{([^{}]+)\}`)
)

type Pack struct {
	Version    int         `json:"version"`
	OutputDir  string      `json:"outputDir,omitempty"`
	DeployDir  string      `json:"deployDir,omitempty"`
	References []Reference `json:"references,omitempty"`
	Objects    []Object    `json:"objects"`
}

type Reference struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type Object struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Size        Size        `json:"size"`
	References  []Reference `json:"references,omitempty"`
	Variants    []Variant   `json:"variants,omitempty"`
	Animations  []Animation `json:"animations,omitempty"`
	Deploy      Deploy      `json:"deploy,omitempty"`
}

type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Variant struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	References  []Reference    `json:"references,omitempty"`
	Values      []VariantValue `json:"values"`
}

type VariantValue struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	References  []Reference `json:"references,omitempty"`
}

type Animation struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	References  []Reference `json:"references,omitempty"`
	Frames      []Frame     `json:"frames"`
}

type Frame struct {
	ID          string      `json:"id,omitempty"`
	Description string      `json:"description"`
	References  []Reference `json:"references,omitempty"`
}

type Deploy struct {
	PathTemplate string `json:"pathTemplate,omitempty"`
}

func Load(dir string) (*Pack, string, error) {
	themeBytes, err := os.ReadFile(filepath.Join(dir, "THEME.md"))
	if err != nil {
		return nil, "", fmt.Errorf("read THEME.md: %w", err)
	}
	file, err := os.Open(filepath.Join(dir, "sprites.json"))
	if err != nil {
		return nil, "", fmt.Errorf("open sprites.json: %w", err)
	}
	defer file.Close()
	p, err := Decode(file)
	if err != nil {
		return nil, "", err
	}
	if err := Validate(dir, p); err != nil {
		return nil, "", err
	}
	return p, string(themeBytes), nil
}

func Decode(r io.Reader) (*Pack, error) {
	var p Pack
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode sprites.json: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("decode sprites.json: trailing JSON content")
	}
	return &p, nil
}

func Validate(dir string, p *Pack) error {
	if p.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", p.Version)
	}
	if len(p.Objects) == 0 {
		return errors.New("objects must contain at least one object")
	}
	if err := validateReferences(dir, "pack", p.References); err != nil {
		return err
	}
	objects := map[string]struct{}{}
	for _, obj := range p.Objects {
		if err := validateID("object", obj.ID); err != nil {
			return err
		}
		if _, exists := objects[obj.ID]; exists {
			return fmt.Errorf("duplicate object id %q", obj.ID)
		}
		objects[obj.ID] = struct{}{}
		if strings.TrimSpace(obj.Description) == "" {
			return fmt.Errorf("object %q description is required", obj.ID)
		}
		if obj.Size.Width <= 0 || obj.Size.Height <= 0 {
			return fmt.Errorf("object %q size must be positive", obj.ID)
		}
		if err := validateReferences(dir, "object "+obj.ID, obj.References); err != nil {
			return err
		}
		if err := validateDeployTemplate(obj); err != nil {
			return fmt.Errorf("object %q deploy template: %w", obj.ID, err)
		}
		if err := validateVariants(dir, obj); err != nil {
			return err
		}
		if err := validateAnimations(dir, obj); err != nil {
			return err
		}
	}
	return nil
}

func validateDeployTemplate(obj Object) error {
	template := DeployTemplate(obj)
	if err := validateRelativePath(template); err != nil {
		return err
	}
	axisIDs := map[string]struct{}{}
	for _, variant := range obj.Variants {
		axisIDs[variant.ID] = struct{}{}
	}
	withoutPlaceholders := placeholderPattern.ReplaceAllString(template, "")
	if strings.ContainsAny(withoutPlaceholders, "{}") {
		return fmt.Errorf("malformed deploy placeholder in %q", template)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		name := match[1]
		switch {
		case name == "target" || name == "object" || name == "animation" || name == "frame":
			continue
		case strings.HasPrefix(name, "variant."):
			axis := strings.TrimPrefix(name, "variant.")
			if axis == "" {
				return fmt.Errorf("malformed deploy placeholder %q", match[0])
			}
			if _, exists := axisIDs[axis]; exists {
				continue
			}
		}
		return fmt.Errorf("unknown deploy placeholder %q", match[0])
	}
	return nil
}

func validateVariants(dir string, obj Object) error {
	seen := map[string]struct{}{}
	for _, variant := range obj.Variants {
		if err := validateID("variant", variant.ID); err != nil {
			return fmt.Errorf("object %q: %w", obj.ID, err)
		}
		if _, exists := seen[variant.ID]; exists {
			return fmt.Errorf("object %q duplicate variant id %q", obj.ID, variant.ID)
		}
		seen[variant.ID] = struct{}{}
		if strings.TrimSpace(variant.Description) == "" {
			return fmt.Errorf("object %q variant %q description is required", obj.ID, variant.ID)
		}
		if len(variant.Values) == 0 {
			return fmt.Errorf("object %q variant %q must contain values", obj.ID, variant.ID)
		}
		if err := validateReferences(dir, "variant "+variant.ID, variant.References); err != nil {
			return err
		}
		values := map[string]struct{}{}
		for _, value := range variant.Values {
			if err := validateID("variant value", value.ID); err != nil {
				return fmt.Errorf("object %q variant %q: %w", obj.ID, variant.ID, err)
			}
			if _, exists := values[value.ID]; exists {
				return fmt.Errorf("object %q variant %q duplicate value id %q", obj.ID, variant.ID, value.ID)
			}
			values[value.ID] = struct{}{}
			if strings.TrimSpace(value.Description) == "" {
				return fmt.Errorf("object %q variant %q value %q description is required", obj.ID, variant.ID, value.ID)
			}
			if err := validateReferences(dir, "variant value "+value.ID, value.References); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAnimations(dir string, obj Object) error {
	seen := map[string]struct{}{}
	for _, animation := range obj.Animations {
		if err := validateID("animation", animation.ID); err != nil {
			return fmt.Errorf("object %q: %w", obj.ID, err)
		}
		if _, exists := seen[animation.ID]; exists {
			return fmt.Errorf("object %q duplicate animation id %q", obj.ID, animation.ID)
		}
		seen[animation.ID] = struct{}{}
		if strings.TrimSpace(animation.Description) == "" {
			return fmt.Errorf("object %q animation %q description is required", obj.ID, animation.ID)
		}
		if len(animation.Frames) == 0 {
			return fmt.Errorf("object %q animation %q must contain frames", obj.ID, animation.ID)
		}
		if err := validateReferences(dir, "animation "+animation.ID, animation.References); err != nil {
			return err
		}
		frameIDs := map[string]struct{}{}
		for i, frame := range animation.Frames {
			id := FrameID(i, frame)
			if err := validateID("frame", id); err != nil {
				return fmt.Errorf("object %q animation %q: %w", obj.ID, animation.ID, err)
			}
			if _, exists := frameIDs[id]; exists {
				return fmt.Errorf("object %q animation %q duplicate frame id %q", obj.ID, animation.ID, id)
			}
			frameIDs[id] = struct{}{}
			if strings.TrimSpace(frame.Description) == "" {
				return fmt.Errorf("object %q animation %q frame %q description is required", obj.ID, animation.ID, id)
			}
			if err := validateReferences(dir, "frame "+id, frame.References); err != nil {
				return err
			}
		}
	}
	return nil
}

func FrameID(index int, frame Frame) string {
	if frame.ID != "" {
		return frame.ID
	}
	return fmt.Sprintf("%02d", index)
}

func OutputDir(p *Pack) string {
	if p.OutputDir != "" {
		return p.OutputDir
	}
	return DefaultOutputDir
}

func DeployTemplate(obj Object) string {
	if obj.Deploy.PathTemplate != "" {
		return obj.Deploy.PathTemplate
	}
	if len(obj.Animations) == 0 {
		return DefaultStaticDeployTemplate
	}
	return DefaultAnimatedDeployTemplate
}

func validateID(kind, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s id %q must match %s", kind, id, idPattern.String())
	}
	return nil
}

func validateReferences(dir, owner string, refs []Reference) error {
	for _, ref := range refs {
		if strings.TrimSpace(ref.Path) == "" {
			return fmt.Errorf("%s reference path is required", owner)
		}
		if strings.TrimSpace(ref.Description) == "" {
			return fmt.Errorf("%s reference %q description is required", owner, ref.Path)
		}
		if err := validateRelativePath(ref.Path); err != nil {
			return fmt.Errorf("%s reference %q: %w", owner, ref.Path, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ref.Path)); err != nil {
			return fmt.Errorf("%s reference %q: %w", owner, ref.Path, err)
		}
	}
	return nil
}

func validateRelativePath(path string) error {
	if filepath.IsAbs(path) {
		return errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("path traversal is not allowed")
	}
	return nil
}
