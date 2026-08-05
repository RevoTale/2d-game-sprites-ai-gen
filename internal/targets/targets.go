// Package targets expands validated sprite packs into deterministic generation targets.
package targets

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
)

const StyleGuideTargetID = pack.StyleGuideObjectID

type Target struct {
	ID               string
	ObjectID         string
	ObjectKind       string
	ObjectDesc       string
	Archetype        string
	Family           string
	Style            pack.Style
	MagicSources     []pack.MagicSource
	IdentityLocks    []string
	RenderMode       string
	RegistrationMode string
	DirectionID      string
	DirectionDesc    string
	DirectionRefPath string
	DirectionRefDesc string
	AnimationID      string
	AnimationDesc    string
	AnimationIndex   int
	FrameID          string
	FrameDesc        string
	FrameIndex       int
	SetPartID        string
	SetPartRole      string
	SetPartDesc      string
	Size             pack.Size
	Inputs           []conditioning.Input
	DeployTemplate   string
	Prompt           string
}

type Filter struct {
	Object  string
	Exclude map[string]bool
}

func Expand(p *pack.Pack) ([]Target, error) {
	var out []Target
	for _, obj := range p.Objects {
		if obj.Kind == pack.KindStatic {
			out = append(out, makeTarget(p, obj, nil, pack.Animation{}, -1, -1, pack.Frame{}))
			continue
		}
		if obj.Kind == pack.KindStaticSet {
			for _, part := range obj.Parts {
				out = append(out, makeStaticSetTarget(p, obj, part))
			}
			continue
		}
		for directionIndex := range obj.Directions {
			direction := &obj.Directions[directionIndex]
			for animationIndex, animation := range obj.Animations {
				for frameIndex, frame := range animation.Frames {
					out = append(out, makeTarget(
						p,
						obj,
						direction,
						animation,
						animationIndex,
						frameIndex,
						frame,
					))
				}
			}
		}
	}
	return out, nil
}

func StyleGuideTarget(p *pack.Pack) Target {
	target := Target{
		ID:               StyleGuideTargetID,
		ObjectID:         StyleGuideTargetID,
		ObjectKind:       StyleGuideTargetID,
		ObjectDesc:       p.StyleGuide.Description,
		Style:            p.Style,
		RenderMode:       pack.RenderModeOpaqueTile,
		RegistrationMode: pack.RegistrationModeCanvas,
		Size:             p.StyleGuide.Size,
		Inputs:           roleInputs(conditioning.RoleStyle, p.StyleGuide.Inputs),
		DeployTemplate:   p.StyleGuide.Deploy.Path,
	}
	target.Prompt = StyleGuidePrompt(p.Style, p.StyleGuide)
	return target
}

func Match(target Target, filter Filter) bool {
	if filter.Exclude[target.ObjectID] {
		return false
	}
	if filter.Object != "" && target.ObjectID != filter.Object {
		return false
	}
	return true
}

func FilterTargets(all []Target, filter Filter) []Target {
	var out []Target
	for _, target := range all {
		if Match(target, filter) {
			out = append(out, target)
		}
	}
	return out
}

func Select(all []Target, filter Filter) ([]Target, error) {
	selected := FilterTargets(all, filter)
	if len(selected) == 0 {
		return nil, errors.New("no targets matched selector")
	}
	return selected, nil
}

func AtomicGroups(selected []Target) [][]Target {
	indexes := map[string]int{}
	groups := make([][]Target, 0, len(selected))
	for _, target := range selected {
		key := "static\x00" + target.ID
		if target.AnimationID != "" {
			key = "unit\x00" + target.ObjectID
		} else if target.ObjectKind == pack.KindStaticSet {
			key = "static-set\x00" + target.ObjectID
		}
		index, ok := indexes[key]
		if !ok {
			index = len(groups)
			indexes[key] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], target)
	}
	return groups
}

func makeStaticSetTarget(p *pack.Pack, obj pack.Object, part pack.StaticSetPart) Target {
	partObject := obj
	partObject.Size = part.Size
	partObject.Deploy = part.Deploy
	partObject.Parts = nil
	target := makeTarget(p, partObject, nil, pack.Animation{}, -1, -1, pack.Frame{})
	target.ID = obj.ID + "-part-" + part.ID
	target.SetPartID = part.ID
	target.SetPartRole = part.Role
	target.SetPartDesc = part.Description
	target.Prompt = BuildPrompt(target)
	return target
}

func makeTarget(
	p *pack.Pack,
	obj pack.Object,
	direction *pack.Direction,
	animation pack.Animation,
	animationIndex, frameIndex int,
	frame pack.Frame,
) Target {
	parts := []string{obj.ID}
	inputs := []conditioning.Input{{
		ID:          p.Style.Reference.ID,
		Role:        conditioning.RoleStyle,
		Authority:   "approved-style-guide",
		SourcePath:  p.Style.Reference.Path,
		Path:        p.Style.Reference.Path,
		Description: p.Style.Reference.Description,
		Required:    true,
	}}
	inputs = append(inputs, roleInputs(conditioning.RoleIdentity, obj.References)...)
	if animation.ID != "" {
		parts = append(parts, animation.ID)
	}
	if direction != nil {
		parts = append(parts, "direction-"+direction.ID)
	}
	frameID := ""
	if frameIndex >= 0 {
		frameID = pack.FrameID(frameIndex, frame)
		parts = append(parts, frameID)
	}
	target := Target{
		ID:               strings.Join(parts, "__"),
		ObjectID:         obj.ID,
		ObjectKind:       obj.Kind,
		ObjectDesc:       obj.Description,
		Archetype:        obj.Archetype,
		Family:           obj.Family,
		Style:            p.Style,
		MagicSources:     cloneMagicSources(obj.MagicSources),
		IdentityLocks:    append([]string(nil), obj.IdentityLocks...),
		RenderMode:       pack.EffectiveRenderMode(obj),
		RegistrationMode: pack.EffectiveRegistrationMode(obj),
		AnimationID:      animation.ID,
		AnimationDesc:    animation.Description,
		AnimationIndex:   animationIndex,
		FrameID:          frameID,
		FrameDesc:        frame.Description,
		FrameIndex:       frameIndex,
		Size:             obj.Size,
		Inputs:           inputs,
		DeployTemplate:   pack.DeployTemplate(obj),
	}
	if direction != nil {
		target.DirectionID = direction.ID
		target.DirectionDesc = direction.Description
		target.DirectionRefPath = direction.Reference.Path
		target.DirectionRefDesc = direction.Reference.Description
	}
	target.Prompt = BuildPrompt(target)
	return target
}

func roleInputs(role conditioning.Role, refs []pack.Reference) []conditioning.Input {
	out := make([]conditioning.Input, 0, len(refs))
	for _, ref := range refs {
		out = append(out, conditioning.Input{
			ID:          ref.ID,
			Role:        role,
			Authority:   role.String(),
			SourcePath:  ref.Path,
			Path:        ref.Path,
			Description: ref.Description,
			Required:    ref.Required,
		})
	}
	return out
}

func BuildPrompt(target Target) string {
	var b strings.Builder
	writeStyleFacts(&b, target.Style)
	fmt.Fprintf(&b, "\n# Object\n%s\n", target.ObjectDesc)
	b.WriteString(MagicSourceFacts(target.MagicSources))
	if target.SetPartID != "" {
		fmt.Fprintf(&b, "\n# Set part: %s\nRole: %s\n%s\n", target.SetPartID, target.SetPartRole, target.SetPartDesc)
	}
	if target.Archetype != "" {
		writeRuleSet(&b, "Unit archetype: "+target.Archetype, target.Style.Units.Archetypes[target.Archetype])
	}
	if target.Family != "" {
		writeRuleSet(&b, "Terrain family: "+target.Family, target.Style.Terrain.Families[target.Family])
	}
	if target.DirectionID != "" {
		fmt.Fprintf(&b, "\n# Direction: %s\n%s\n", target.DirectionID, target.DirectionDesc)
	}
	if target.AnimationID != "" {
		fmt.Fprintf(&b, "\n# Animation: %s\n%s\n", target.AnimationID, target.AnimationDesc)
	}
	if target.FrameID != "" {
		fmt.Fprintf(&b, "\n# Frame: %s\n%s\n", target.FrameID, target.FrameDesc)
	}
	return b.String()
}

func cloneMagicSources(sources *[]pack.MagicSource) []pack.MagicSource {
	if sources == nil {
		return []pack.MagicSource{}
	}
	cloned := make([]pack.MagicSource, len(*sources))
	for index, source := range *sources {
		cloned[index] = source
		cloned[index].Limits = append([]string{}, source.Limits...)
	}
	return cloned
}

// MagicSourceFacts renders the complete causal appearance contract for use by
// both static and animated provider prompts.
func MagicSourceFacts(sources []pack.MagicSource) string {
	var b strings.Builder
	if len(sources) == 0 {
		b.WriteString("\n# Supernatural sources\n")
		b.WriteString("None declared. Do not invent glow, runes, magical filigree, detached energy, or unnatural color flow.\n")
		return b.String()
	}
	for _, source := range sources {
		fmt.Fprintf(&b, "\n# Supernatural source: %s\n", source.ID)
		fmt.Fprintf(&b, "%s\n", source.Description)
		fmt.Fprintf(&b, "Location: %s\n", source.Location)
		fmt.Fprintf(&b, "Palette: %s\n", source.Palette)
		fmt.Fprintf(&b, "Expression: %s\n", source.Expression)
		writeStrings(&b, "Limits", source.Limits)
	}
	return b.String()
}

func StyleGuidePrompt(style pack.Style, guide pack.StyleGuide) string {
	var b strings.Builder
	writeStyleFacts(&b, style)
	fmt.Fprintf(&b, "\n# Original style guide\n%s\n", guide.Description)
	return b.String()
}

func StyleFacts(style pack.Style) string {
	var b strings.Builder
	writeStyleFacts(&b, style)
	return b.String()
}

// UnitStyleFacts omits terrain-only rules from character requests. The full
// style still remains authoritative in sprites.json and in the style guide.
func UnitStyleFacts(style pack.Style) string {
	var b strings.Builder
	writeSharedStyleFacts(&b, style)
	writeStrings(&b, "Unit rules", style.Units.Common)
	writeStrings(&b, "Forbidden", style.Forbidden)
	return b.String()
}

func writeStyleFacts(b *strings.Builder, style pack.Style) {
	writeSharedStyleFacts(b, style)
	writeStrings(b, "Unit rules", style.Units.Common)
	writeStrings(b, "Terrain rules", style.Terrain.Common)
	writeStrings(b, "Forbidden", style.Forbidden)
}

func writeSharedStyleFacts(b *strings.Builder, style pack.Style) {
	fmt.Fprintf(b, "# Style: %s\n%s\n", style.ID, style.Description)
	writeStrings(b, "Principles", style.Principles)
	fmt.Fprintf(
		b,
		"\nPalette: maximum %d colors; %s matching; %s alpha; %s dithering.\n",
		style.Palette.MaxColors,
		style.Palette.ColorSpace,
		style.Palette.Alpha,
		style.Palette.Dithering,
	)
	writeStrings(b, "Contrast hierarchy", style.ContrastHierarchy)
}

func writeRuleSet(b *strings.Builder, title string, rules pack.VisualRuleSet) {
	fmt.Fprintf(b, "\n# %s\n%s\n", title, rules.Description)
	writeStrings(b, "Rules", rules.Rules)
}

func writeStrings(b *strings.Builder, title string, values []string) {
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func DeployPath(deployDir string, target Target) (string, error) {
	path := target.DeployTemplate
	path = strings.ReplaceAll(path, "{target}", target.ID)
	path = strings.ReplaceAll(path, "{object}", target.ObjectID)
	path = strings.ReplaceAll(path, "{animation}", target.AnimationID)
	path = strings.ReplaceAll(path, "{frame}", target.FrameID)
	path = strings.ReplaceAll(path, "{direction}", target.DirectionID)
	if strings.ContainsAny(path, "{}") {
		return "", fmt.Errorf("deploy path %q contains unresolved placeholders", path)
	}
	if filepath.IsAbs(path) || path == "." || strings.HasPrefix(filepath.Clean(path), "..") {
		return "", fmt.Errorf("deploy path %q is not safely relative", path)
	}
	if strings.TrimSpace(deployDir) == "" {
		return "", errors.New("deploy directory is required")
	}
	return filepath.Join(deployDir, path), nil
}

func ObjectIDs(all []Target) []string {
	seen := map[string]bool{}
	var ids []string
	for _, target := range all {
		if !seen[target.ObjectID] {
			seen[target.ObjectID] = true
			ids = append(ids, target.ObjectID)
		}
	}
	sort.Strings(ids)
	return ids
}
