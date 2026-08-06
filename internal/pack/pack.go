// Package pack loads and validates the JSON-only sprite generation contract.
package pack

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	Version                       = 6
	DefaultOutputDir              = "output"
	DefaultStaticDeployTemplate   = "sprites/{target}.png"
	DefaultAnimatedDeployTemplate = "frames/units/{object}/{animation}/{direction}/{frame}.png"
	KindAnimated                  = "animated"
	KindStatic                    = "static"
	KindStaticSet                 = "static-set"
	RenderModeIsolated            = "isolated"
	RenderModeOpaqueTile          = "opaque-tile"
	RenderModeMaterialSwatch      = "material-swatch"
	RenderModeTransparentOverlay  = "transparent-overlay"
	RegistrationModeGrounded      = "grounded"
	RegistrationModeCentered      = "centered"
	RegistrationModeCanvas        = "canvas"
	AnimatedFrameSize             = 384
	LegacyAnimatedReferenceSize   = 320
	StyleGuideObjectID            = "style-guide"
	ScaleClassStandardHumanoid    = "standard-humanoid"
	ScaleClassReferenceStable     = "reference-stable"
)

var (
	idPattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	placeholderPattern = regexp.MustCompile(`\{([^{}]+)\}`)
	requiredDirections = []string{"down", "up", "right"}
)

type Pack struct {
	Version    int        `json:"version"`
	OutputDir  string     `json:"outputDir,omitempty"`
	DeployDir  string     `json:"deployDir,omitempty"`
	Style      Style      `json:"style"`
	StyleGuide StyleGuide `json:"styleGuide"`
	Objects    []Object   `json:"objects"`
}

type Style struct {
	ID                string         `json:"id"`
	Description       string         `json:"description"`
	Principles        []string       `json:"principles"`
	Palette           Palette        `json:"palette"`
	ContrastHierarchy []string       `json:"contrastHierarchy"`
	Units             UnitStyle      `json:"units"`
	Terrain           TerrainStyle   `json:"terrain"`
	Forbidden         []string       `json:"forbidden"`
	Reference         StyleReference `json:"reference"`
}

type Palette struct {
	MaxColors  int    `json:"maxColors"`
	ColorSpace string `json:"colorSpace"`
	Alpha      string `json:"alpha"`
	Dithering  string `json:"dithering"`
}

type UnitStyle struct {
	Common     []string                 `json:"common"`
	Archetypes map[string]VisualRuleSet `json:"archetypes"`
}

type TerrainStyle struct {
	Common   []string                 `json:"common"`
	Families map[string]VisualRuleSet `json:"families"`
}

type VisualRuleSet struct {
	Description string   `json:"description"`
	ScaleClass  string   `json:"scaleClass,omitempty"`
	Rules       []string `json:"rules"`
}

type StyleReference struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type StyleGuide struct {
	Description string      `json:"description"`
	Size        Size        `json:"size"`
	Inputs      []Reference `json:"inputs"`
	Deploy      GuideDeploy `json:"deploy"`
}

type GuideDeploy struct {
	Path string `json:"path"`
}

type Reference struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type Object struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Archetype     string          `json:"archetype,omitempty"`
	Family        string          `json:"family,omitempty"`
	Description   string          `json:"description"`
	MagicSources  *[]MagicSource  `json:"magicSources"`
	IdentityLocks []string        `json:"identityLocks,omitempty"`
	RenderMode    string          `json:"renderMode,omitempty"`
	Registration  string          `json:"registration"`
	Size          Size            `json:"size"`
	References    []Reference     `json:"references,omitempty"`
	Directions    []Direction     `json:"directions,omitempty"`
	Animations    []Animation     `json:"animations,omitempty"`
	Deploy        Deploy          `json:"deploy,omitempty"`
	Parts         []StaticSetPart `json:"parts,omitempty"`
}

// MagicSource explains one bounded supernatural feature. Its explicit cause
// prevents generic prompt styling from inventing unrelated magical ornament.
type MagicSource struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Palette     string   `json:"palette"`
	Expression  string   `json:"expression"`
	Limits      []string `json:"limits"`
}

// StaticSetPart is one deployable member of a visually coupled atomic set.
// Its role and logical size are pack facts; provider-board geometry remains
// generator-owned.
type StaticSetPart struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Description string `json:"description"`
	Size        Size   `json:"size"`
	Deploy      Deploy `json:"deploy"`
}

type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Direction struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Reference   DirectionReference `json:"reference"`
}

type DirectionReference struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

type Animation struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	Frames      []Frame `json:"frames"`
}

type Frame struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type Deploy struct {
	PathTemplate string `json:"pathTemplate,omitempty"`
}

func Load(dir string) (*Pack, error) {
	file, err := os.Open(filepath.Join(dir, "sprites.json"))
	if err != nil {
		return nil, fmt.Errorf("open sprites.json: %w", err)
	}
	defer file.Close()
	p, err := Decode(file)
	if err != nil {
		return nil, err
	}
	if err := Validate(dir, p); err != nil {
		return nil, err
	}
	return p, nil
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
	if p.Version >= 1 && p.Version < Version {
		return fmt.Errorf("sprites.json v%d is unsupported; migrate the pack to v6", p.Version)
	}
	if p.Version != Version {
		return fmt.Errorf("sprites.json v%d is unsupported; expected v6", p.Version)
	}
	referenceIDs := map[string]string{}
	if err := validateStyle(dir, p, referenceIDs); err != nil {
		return err
	}
	if len(p.Objects) == 0 {
		return errors.New("objects must contain at least one object")
	}
	objects := map[string]struct{}{}
	for _, obj := range p.Objects {
		if err := validateObject(dir, p, obj, referenceIDs, objects); err != nil {
			return err
		}
	}
	return nil
}

func validateStyle(dir string, p *Pack, referenceIDs map[string]string) error {
	style := p.Style
	if err := validateID("style", style.ID); err != nil {
		return err
	}
	if err := requireText("style description", style.Description); err != nil {
		return err
	}
	if err := requireTexts("style principles", style.Principles); err != nil {
		return err
	}
	if style.Palette.MaxColors != 32 ||
		style.Palette.ColorSpace != "linear-srgb" ||
		style.Palette.Alpha != "binary" ||
		style.Palette.Dithering != "none" {
		return errors.New("style palette must use maxColors=32, colorSpace=linear-srgb, alpha=binary, and dithering=none")
	}
	if err := requireTexts("style contrastHierarchy", style.ContrastHierarchy); err != nil {
		return err
	}
	if err := requireTexts("style units common", style.Units.Common); err != nil {
		return err
	}
	if err := validateUnitArchetypes(style.Units.Archetypes); err != nil {
		return err
	}
	if err := requireTexts("style terrain common", style.Terrain.Common); err != nil {
		return err
	}
	if err := validateRuleSets("terrain family", style.Terrain.Families); err != nil {
		return err
	}
	if err := requireTexts("style forbidden", style.Forbidden); err != nil {
		return err
	}
	if err := validateStyleReference(dir, style.Reference, referenceIDs); err != nil {
		return err
	}
	guide := p.StyleGuide
	if err := requireText("styleGuide description", guide.Description); err != nil {
		return err
	}
	if guide.Size.Width != 1536 || guide.Size.Height != 1024 {
		return fmt.Errorf("styleGuide size must be 1536x1024, got %dx%d", guide.Size.Width, guide.Size.Height)
	}
	if len(guide.Inputs) == 0 {
		return errors.New("styleGuide inputs must contain at least one original repository reference")
	}
	if err := validateReferences(dir, "styleGuide", "style", guide.Inputs, referenceIDs); err != nil {
		return err
	}
	if err := validateRelativePath(guide.Deploy.Path); err != nil {
		return fmt.Errorf("styleGuide deploy path: %w", err)
	}
	cleanGuidePath := filepath.Clean(guide.Deploy.Path)
	styleRoot := filepath.Clean(filepath.Join("references", "style"))
	if cleanGuidePath != styleRoot && !strings.HasPrefix(cleanGuidePath, styleRoot+string(filepath.Separator)) {
		return fmt.Errorf("styleGuide deploy path %q must stay under references/style", guide.Deploy.Path)
	}
	if cleanGuidePath != filepath.Clean(style.Reference.Path) {
		return errors.New("style reference path must equal styleGuide deploy path")
	}
	return nil
}

func validateStyleReference(dir string, ref StyleReference, referenceIDs map[string]string) error {
	if err := validateID("style reference", ref.ID); err != nil {
		return err
	}
	if previous, exists := referenceIDs[ref.ID]; exists {
		return fmt.Errorf("duplicate reference id %q in %s and style", ref.ID, previous)
	}
	referenceIDs[ref.ID] = "style"
	if err := requireText("style reference path", ref.Path); err != nil {
		return err
	}
	if err := requireText("style reference description", ref.Description); err != nil {
		return err
	}
	if err := validateRelativePath(ref.Path); err != nil {
		return fmt.Errorf("style reference %q: %w", ref.Path, err)
	}
	path := filepath.Join(dir, ref.Path)
	size, err := pngDimensions(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("style reference %q: %w", ref.Path, err)
	}
	if size != (Size{Width: 1536, Height: 1024}) {
		return fmt.Errorf("style reference %q is %dx%d, expected 1536x1024", ref.Path, size.Width, size.Height)
	}
	return nil
}

func validateRuleSets(kind string, values map[string]VisualRuleSet) error {
	if len(values) == 0 {
		return fmt.Errorf("%ss must contain at least one entry", kind)
	}
	for id, value := range values {
		if err := validateID(kind, id); err != nil {
			return err
		}
		if err := requireText(kind+" "+id+" description", value.Description); err != nil {
			return err
		}
		if err := requireTexts(kind+" "+id+" rules", value.Rules); err != nil {
			return err
		}
	}
	return nil
}

func validateUnitArchetypes(values map[string]VisualRuleSet) error {
	if err := validateRuleSets("unit archetype", values); err != nil {
		return err
	}
	for id, value := range values {
		if value.ScaleClass == "" {
			return fmt.Errorf("unit archetype %q scaleClass is required", id)
		}
		switch value.ScaleClass {
		case ScaleClassStandardHumanoid, ScaleClassReferenceStable:
		default:
			return fmt.Errorf(
				"unit archetype %q uses unknown scaleClass %q",
				id,
				value.ScaleClass,
			)
		}
	}
	return nil
}

func validateObject(
	dir string,
	p *Pack,
	obj Object,
	referenceIDs map[string]string,
	objects map[string]struct{},
) error {
	if err := validateID("object", obj.ID); err != nil {
		return err
	}
	if obj.ID == StyleGuideObjectID {
		return fmt.Errorf("object id %q is reserved for style-guide bootstrap", obj.ID)
	}
	if _, exists := objects[obj.ID]; exists {
		return fmt.Errorf("duplicate object id %q", obj.ID)
	}
	objects[obj.ID] = struct{}{}
	if err := requireText("object "+obj.ID+" description", obj.Description); err != nil {
		return err
	}
	if err := validateMagicSources(obj); err != nil {
		return err
	}
	if obj.Kind != KindStaticSet && (obj.Size.Width <= 0 || obj.Size.Height <= 0) {
		return fmt.Errorf("object %q size must be positive", obj.ID)
	}
	if err := validateRenderMode(obj); err != nil {
		return err
	}
	if err := validateRegistration(obj); err != nil {
		return err
	}
	if err := validateReferences(dir, "object "+obj.ID, "identity", obj.References, referenceIDs); err != nil {
		return err
	}
	switch obj.Kind {
	case KindAnimated:
		if len(obj.Parts) != 0 {
			return fmt.Errorf("animated object %q must not define parts", obj.ID)
		}
		if _, ok := p.Style.Units.Archetypes[obj.Archetype]; !ok {
			return fmt.Errorf("animated object %q uses unknown archetype %q", obj.ID, obj.Archetype)
		}
		if obj.Family != "" {
			return fmt.Errorf("animated object %q must not define family", obj.ID)
		}
		if err := requireTexts("animated object "+obj.ID+" identityLocks", obj.IdentityLocks); err != nil {
			return err
		}
		if err := validateDirections(dir, p.DeployDir, obj, referenceIDs); err != nil {
			return err
		}
		if len(obj.Animations) == 0 {
			return fmt.Errorf("animated object %q must define animations", obj.ID)
		}
	case KindStatic:
		if len(obj.Parts) != 0 {
			return fmt.Errorf("static object %q must not define parts", obj.ID)
		}
		if _, ok := p.Style.Terrain.Families[obj.Family]; !ok {
			return fmt.Errorf("static object %q uses unknown terrain family %q", obj.ID, obj.Family)
		}
		if obj.Archetype != "" {
			return fmt.Errorf("static object %q must not define archetype", obj.ID)
		}
		if len(obj.Directions) != 0 || len(obj.Animations) != 0 {
			return fmt.Errorf("static object %q must not define directions or animations", obj.ID)
		}
	case KindStaticSet:
		if _, ok := p.Style.Terrain.Families[obj.Family]; !ok {
			return fmt.Errorf("static set %q uses unknown terrain family %q", obj.ID, obj.Family)
		}
		if obj.Archetype != "" {
			return fmt.Errorf("static set %q must not define archetype", obj.ID)
		}
		if obj.Size != (Size{}) {
			return fmt.Errorf("static set %q must not define object size", obj.ID)
		}
		if obj.Deploy != (Deploy{}) {
			return fmt.Errorf("static set %q must not define object deploy", obj.ID)
		}
		if len(obj.Directions) != 0 || len(obj.Animations) != 0 {
			return fmt.Errorf("static set %q must not define directions or animations", obj.ID)
		}
		if err := validateStaticSet(obj); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"object %q kind %q is unsupported; expected %q, %q, or %q",
			obj.ID,
			obj.Kind,
			KindAnimated,
			KindStatic,
			KindStaticSet,
		)
	}
	if err := validateAnimations(obj); err != nil {
		return err
	}
	if obj.Kind != KindStaticSet {
		if err := validateDeployTemplate(obj); err != nil {
			return fmt.Errorf("object %q deploy template: %w", obj.ID, err)
		}
	}
	return nil
}

func validateStaticSet(obj Object) error {
	if len(obj.Parts) < 2 {
		return fmt.Errorf("static set %q must contain at least two parts", obj.ID)
	}
	seen := make(map[string]struct{}, len(obj.Parts))
	for _, part := range obj.Parts {
		if err := validateID("static set part", part.ID); err != nil {
			return fmt.Errorf("static set %q: %w", obj.ID, err)
		}
		if _, exists := seen[part.ID]; exists {
			return fmt.Errorf("static set %q duplicate part id %q", obj.ID, part.ID)
		}
		seen[part.ID] = struct{}{}
		if err := requireText("static set "+obj.ID+" part "+part.ID+" role", part.Role); err != nil {
			return err
		}
		if err := requireText("static set "+obj.ID+" part "+part.ID+" description", part.Description); err != nil {
			return err
		}
		if part.Size.Width <= 0 || part.Size.Height <= 0 {
			return fmt.Errorf("static set %q part %q size must be positive", obj.ID, part.ID)
		}
		if err := requireText("static set "+obj.ID+" part "+part.ID+" deploy pathTemplate", part.Deploy.PathTemplate); err != nil {
			return err
		}
		partObject := Object{Kind: KindStatic, Deploy: part.Deploy}
		if err := validateDeployTemplate(partObject); err != nil {
			return fmt.Errorf("static set %q part %q deploy template: %w", obj.ID, part.ID, err)
		}
	}
	return nil
}

func validateMagicSources(obj Object) error {
	if obj.MagicSources == nil {
		return fmt.Errorf("object %q magicSources is required", obj.ID)
	}
	seen := make(map[string]struct{}, len(*obj.MagicSources))
	for _, source := range *obj.MagicSources {
		if err := validateID("magic source", source.ID); err != nil {
			return fmt.Errorf("object %q: %w", obj.ID, err)
		}
		if _, exists := seen[source.ID]; exists {
			return fmt.Errorf("object %q duplicate magic source id %q", obj.ID, source.ID)
		}
		seen[source.ID] = struct{}{}
		fields := []struct {
			name  string
			value string
		}{
			{name: "description", value: source.Description},
			{name: "location", value: source.Location},
			{name: "palette", value: source.Palette},
			{name: "expression", value: source.Expression},
		}
		for _, field := range fields {
			name := fmt.Sprintf(
				"object %s magic source %s %s",
				obj.ID,
				source.ID,
				field.name,
			)
			if err := requireText(name, field.value); err != nil {
				return err
			}
		}
		name := fmt.Sprintf("object %s magic source %s limits", obj.ID, source.ID)
		if err := requireTexts(name, source.Limits); err != nil {
			return err
		}
	}
	return nil
}

func validateDirections(dir, deployDir string, obj Object, referenceIDs map[string]string) error {
	if len(obj.Directions) != len(requiredDirections) {
		return fmt.Errorf("animated object %q directions must contain exactly down, up, and right", obj.ID)
	}
	paths := map[string]string{}
	for index, expected := range requiredDirections {
		direction := obj.Directions[index]
		if direction.ID != expected {
			return fmt.Errorf(
				"animated object %q direction %d must be %q, got %q",
				obj.ID,
				index,
				expected,
				direction.ID,
			)
		}
		if err := requireText("direction "+direction.ID+" description", direction.Description); err != nil {
			return err
		}
		referenceID := DirectionReferenceID(obj.ID, direction.ID)
		owner := "object " + obj.ID + " direction " + direction.ID
		if previous, exists := referenceIDs[referenceID]; exists {
			return fmt.Errorf("duplicate reference id %q in %s and %s", referenceID, previous, owner)
		}
		referenceIDs[referenceID] = owner
		ref := direction.Reference
		if err := requireText(owner+" reference path", ref.Path); err != nil {
			return err
		}
		if err := requireText(owner+" reference description", ref.Description); err != nil {
			return err
		}
		if err := validateDirectionReferencePath(dir, deployDir, ref.Path); err != nil {
			return fmt.Errorf("%s reference %q: %w", owner, ref.Path, err)
		}
		cleanPath := filepath.Clean(ref.Path)
		if previous, exists := paths[cleanPath]; exists {
			return fmt.Errorf("animated object %q directions %q and %q use duplicate reference %q", obj.ID, previous, direction.ID, ref.Path)
		}
		paths[cleanPath] = direction.ID
		size, err := pngDimensions(filepath.Join(dir, ref.Path))
		if err != nil {
			return fmt.Errorf("%s reference %q: %w", owner, ref.Path, err)
		}
		if !validDirectionReferenceSize(size, obj.Size) {
			return fmt.Errorf(
				"%s reference %q is %dx%d, expected %dx%d or the exact legacy 320x320 transition canvas",
				owner,
				ref.Path,
				size.Width,
				size.Height,
				obj.Size.Width,
				obj.Size.Height,
			)
		}
	}
	return nil
}

func validDirectionReferenceSize(reference, output Size) bool {
	if reference == output {
		return true
	}
	return output == (Size{Width: AnimatedFrameSize, Height: AnimatedFrameSize}) &&
		reference == (Size{Width: LegacyAnimatedReferenceSize, Height: LegacyAnimatedReferenceSize})
}

func validateAnimations(obj Object) error {
	seen := map[string]struct{}{}
	for _, animation := range obj.Animations {
		if err := validateID("animation", animation.ID); err != nil {
			return fmt.Errorf("object %q: %w", obj.ID, err)
		}
		if _, exists := seen[animation.ID]; exists {
			return fmt.Errorf("object %q duplicate animation id %q", obj.ID, animation.ID)
		}
		seen[animation.ID] = struct{}{}
		if err := requireText("animation "+animation.ID+" description", animation.Description); err != nil {
			return err
		}
		if len(animation.Frames) == 0 {
			return fmt.Errorf("object %q animation %q must contain frames", obj.ID, animation.ID)
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
			if err := requireText("frame "+id+" description", frame.Description); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRenderMode(obj Object) error {
	switch obj.RenderMode {
	case "", RenderModeIsolated:
		return nil
	case RenderModeOpaqueTile:
		if obj.Kind == KindAnimated {
			return fmt.Errorf("animated object %q renderMode %q is unsupported", obj.ID, obj.RenderMode)
		}
		return nil
	case RenderModeMaterialSwatch:
		if obj.Kind != KindStaticSet {
			return fmt.Errorf(
				"object %q renderMode %q requires kind %q",
				obj.ID,
				obj.RenderMode,
				KindStaticSet,
			)
		}
		return nil
	case RenderModeTransparentOverlay:
		if obj.Kind != KindStaticSet {
			return fmt.Errorf(
				"object %q renderMode %q requires kind %q",
				obj.ID,
				obj.RenderMode,
				KindStaticSet,
			)
		}
		return nil
	default:
		return fmt.Errorf(
			"object %q renderMode %q is unsupported; expected %q, %q, %q, or %q",
			obj.ID,
			obj.RenderMode,
			RenderModeIsolated,
			RenderModeOpaqueTile,
			RenderModeMaterialSwatch,
			RenderModeTransparentOverlay,
		)
	}
}

func validateRegistration(obj Object) error {
	switch obj.Kind {
	case KindAnimated:
		if obj.Registration != RegistrationModeGrounded && obj.Registration != RegistrationModeCentered {
			return fmt.Errorf(
				"animated object %q registration %q is unsupported; expected %q or %q",
				obj.ID,
				obj.Registration,
				RegistrationModeGrounded,
				RegistrationModeCentered,
			)
		}
	case KindStatic, KindStaticSet:
		renderMode := EffectiveRenderMode(obj)
		if renderMode == RenderModeOpaqueTile || renderMode == RenderModeMaterialSwatch {
			if obj.Registration != RegistrationModeCanvas {
				return fmt.Errorf("opaque-tile object %q registration must be %q", obj.ID, RegistrationModeCanvas)
			}
		} else if renderMode == RenderModeTransparentOverlay {
			if obj.Registration != RegistrationModeCanvas {
				return fmt.Errorf(
					"transparent-overlay object %q registration must be %q",
					obj.ID,
					RegistrationModeCanvas,
				)
			}
		} else if obj.Registration != RegistrationModeGrounded && obj.Registration != RegistrationModeCentered {
			return fmt.Errorf(
				"isolated object %q registration %q is unsupported; expected %q or %q",
				obj.ID,
				obj.Registration,
				RegistrationModeGrounded,
				RegistrationModeCentered,
			)
		}
	}
	return nil
}

func validateDeployTemplate(obj Object) error {
	template := DeployTemplate(obj)
	if err := validateRelativePath(template); err != nil {
		return err
	}
	withoutPlaceholders := placeholderPattern.ReplaceAllString(template, "")
	if strings.ContainsAny(withoutPlaceholders, "{}") {
		return fmt.Errorf("malformed deploy placeholder in %q", template)
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		switch match[1] {
		case "target", "object":
		case "animation", "direction", "frame":
			if obj.Kind != KindAnimated {
				return fmt.Errorf("placeholder %q is valid only for animated objects", match[0])
			}
		default:
			return fmt.Errorf("unknown deploy placeholder %q", match[0])
		}
	}
	return nil
}

func validateReferences(
	dir, owner, expectedRole string,
	refs []Reference,
	referenceIDs map[string]string,
) error {
	for _, ref := range refs {
		if err := validateID(owner+" reference", ref.ID); err != nil {
			return err
		}
		if previous, exists := referenceIDs[ref.ID]; exists {
			return fmt.Errorf("duplicate reference id %q in %s and %s", ref.ID, previous, owner)
		}
		referenceIDs[ref.ID] = owner
		if ref.Role != expectedRole {
			return fmt.Errorf("%s reference %q role must be %q, got %q", owner, ref.ID, expectedRole, ref.Role)
		}
		if err := requireText(owner+" reference path", ref.Path); err != nil {
			return err
		}
		if err := requireText(owner+" reference "+ref.ID+" description", ref.Description); err != nil {
			return err
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

func validateDirectionReferencePath(packDir, deployDir, path string) error {
	if filepath.IsAbs(path) {
		return errors.New("absolute paths are not allowed")
	}
	resolved, err := filepath.Abs(filepath.Join(packDir, path))
	if err != nil {
		return err
	}
	packRoot, err := filepath.Abs(packDir)
	if err != nil {
		return err
	}
	allowed := []string{packRoot}
	if strings.TrimSpace(deployDir) != "" {
		deployRoot, err := filepath.Abs(filepath.Join(packDir, deployDir))
		if err != nil {
			return err
		}
		allowed = append(allowed, deployRoot)
	}
	for _, root := range allowed {
		relative, err := filepath.Rel(root, resolved)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return errors.New("path traversal outside the pack or deploy directory is not allowed")
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

func pngDimensions(path string) (Size, error) {
	file, err := os.Open(path)
	if err != nil {
		return Size{}, err
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil {
		return Size{}, fmt.Errorf("decode PNG: %w", err)
	}
	return Size{Width: config.Width, Height: config.Height}, nil
}

func requireText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func requireTexts(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must contain at least one entry", name)
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] must not be empty", name, index)
		}
	}
	return nil
}

func validateID(kind, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s id %q must match %s", kind, id, idPattern.String())
	}
	return nil
}

func IsAnimated(obj Object) bool {
	return obj.Kind == KindAnimated
}

func EffectiveRegistrationMode(obj Object) string {
	return obj.Registration
}

func EffectiveRenderMode(obj Object) string {
	if obj.RenderMode == "" {
		return RenderModeIsolated
	}
	return obj.RenderMode
}

func DirectionReferenceID(objectID, directionID string) string {
	return "direction-reference-" + objectID + "-" + directionID
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
	if obj.Kind == KindAnimated {
		return DefaultAnimatedDeployTemplate
	}
	return DefaultStaticDeployTemplate
}
