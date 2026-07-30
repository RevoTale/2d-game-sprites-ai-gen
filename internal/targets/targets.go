// Package targets expands validated sprite packs into deterministic generation targets.
package targets

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/conditioning"
	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
)

type Target struct {
	ID               string
	ObjectID         string
	ObjectDesc       string
	IdentityLocks    []string
	RenderMode       string
	RegistrationMode string
	AnimationID      string
	AnimationDesc    string
	AnimationIndex   int
	FrameID          string
	FrameDesc        string
	FrameIndex       int
	Size             pack.Size
	Variants         []VariantSelection
	Inputs           []conditioning.Input
	DeployTemplate   string
	Prompt           string
}

type VariantSelection struct {
	AxisID               string
	ValueID              string
	Description          string
	ReferencePath        string
	ReferenceDescription string
}

type Filter struct {
	Object    string
	Variants  map[string]string
	Animation string
	Frame     string
}

func Expand(p *pack.Pack, theme string) ([]Target, error) {
	var out []Target
	for _, obj := range p.Objects {
		combos := variantCombos(obj.Variants)
		for _, combo := range combos {
			if len(obj.Animations) == 0 {
				out = append(out, makeTarget(p, obj, combo, pack.Animation{}, -1, -1, pack.Frame{}, theme))
				continue
			}
			for animationIndex, animation := range obj.Animations {
				for i, frame := range animation.Frames {
					out = append(out, makeTarget(p, obj, combo, animation, animationIndex, i, frame, theme))
				}
			}
		}
	}
	return out, nil
}

func Match(target Target, filter Filter) bool {
	if filter.Object != "" && target.ObjectID != filter.Object {
		return false
	}
	if filter.Animation != "" && target.AnimationID != filter.Animation {
		return false
	}
	if filter.Frame != "" && target.FrameID != filter.Frame {
		return false
	}
	for axis, value := range filter.Variants {
		found := false
		for _, variant := range target.Variants {
			if variant.AxisID == axis && variant.ValueID == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
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

// Select validates a selector and expands an animated frame match to its
// complete action/variant row. Static matches remain target-atomic.
func Select(all []Target, filter Filter) ([]Target, error) {
	matched := FilterTargets(all, filter)
	if len(matched) == 0 {
		return nil, errors.New("no targets matched selector")
	}
	if filter.Frame == "" {
		return matched, nil
	}
	rows := map[string]bool{}
	statics := map[string]bool{}
	for _, target := range matched {
		if target.AnimationID == "" {
			statics[target.ID] = true
			continue
		}
		rows[RowKey(target)] = true
	}
	selected := make([]Target, 0, len(matched))
	for _, target := range all {
		if statics[target.ID] || (target.AnimationID != "" && rows[RowKey(target)]) {
			selected = append(selected, target)
		}
	}
	return selected, nil
}

// RowKey identifies the action/variant row that owns an animated frame.
func RowKey(target Target) string {
	var b strings.Builder
	b.WriteString(target.ObjectID)
	b.WriteByte('\x00')
	b.WriteString(target.AnimationID)
	for _, variant := range target.Variants {
		b.WriteByte('\x00')
		b.WriteString(variant.AxisID)
		b.WriteByte('=')
		b.WriteString(variant.ValueID)
	}
	return b.String()
}

// AtomicGroups returns static targets individually and animated targets as
// complete rows while preserving selector order.
func AtomicGroups(selected []Target) [][]Target {
	indexes := map[string]int{}
	groups := make([][]Target, 0, len(selected))
	for _, target := range selected {
		key := "static\x00" + target.ID
		if target.AnimationID != "" {
			key = "row\x00" + RowKey(target)
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

func makeTarget(p *pack.Pack, obj pack.Object, variants []variantComboValue, animation pack.Animation, animationIndex, frameIndex int, frame pack.Frame, theme string) Target {
	parts := []string{obj.ID}
	inputs := roleInputs(conditioning.RoleStyle, p.References)
	inputs = append(inputs, roleInputs(conditioning.RoleIdentity, obj.References)...)
	var selections []VariantSelection
	if animation.ID != "" {
		parts = append(parts, animation.ID)
		inputs = append(inputs, roleInputs(conditioning.RolePose, animation.References)...)
	}
	for _, variant := range variants {
		inputs = append(inputs, roleInputs(conditioning.RolePose, variant.AxisRefs)...)
		inputs = append(inputs, roleInputs(conditioning.RolePose, variant.ValueRefs)...)
		selections = append(selections, VariantSelection{
			AxisID:               variant.AxisID,
			ValueID:              variant.ValueID,
			Description:          variant.Description,
			ReferencePath:        variant.ReferencePath,
			ReferenceDescription: variant.ReferenceDescription,
		})
		parts = append(parts, variant.AxisID+"-"+variant.ValueID)
	}
	frameID := ""
	if frameIndex >= 0 {
		frameID = pack.FrameID(frameIndex, frame)
		parts = append(parts, frameID)
		inputs = append(inputs, roleInputs(conditioning.RolePose, frame.References)...)
	}
	target := Target{
		ID:               strings.Join(parts, "__"),
		ObjectID:         obj.ID,
		ObjectDesc:       obj.Description,
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
		Variants:         selections,
		Inputs:           inputs,
		DeployTemplate:   pack.DeployTemplate(obj),
	}
	target.Prompt = BuildPrompt(theme, target)
	return target
}

func roleInputs(role conditioning.Role, refs []pack.Reference) []conditioning.Input {
	out := make([]conditioning.Input, 0, len(refs))
	for _, ref := range refs {
		out = append(out, conditioning.Input{ID: ref.ID, Role: role, Authority: role.String(), SourcePath: ref.Path, Path: ref.Path, Description: ref.Description, Required: ref.Required})
	}
	return out
}

func BuildPrompt(theme string, target Target) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Theme\n%s\n\n# Object\n%s\n\n", strings.TrimSpace(theme), target.ObjectDesc)
	for _, variant := range target.Variants {
		fmt.Fprintf(&b, "# Variant: %s=%s\n%s\n\n", variant.AxisID, variant.ValueID, variant.Description)
	}
	if target.AnimationID != "" {
		fmt.Fprintf(&b, "# Animation: %s\n%s\n\n", target.AnimationID, target.AnimationDesc)
	}
	if target.FrameID != "" {
		fmt.Fprintf(&b, "# Frame: %s\n%s\n\n", target.FrameID, target.FrameDesc)
	}
	if target.RenderMode == pack.RenderModeOpaqueTile {
		fmt.Fprintf(&b, "Generate one independent full-frame texture for a final %dx%d target. Do not compose or crop from a sprite sheet.\n", target.Size.Width, target.Size.Height)
	} else {
		fmt.Fprintf(&b, "Generate one independent sprite image for a final %dx%d target. Do not compose or crop from a sprite sheet.\n", target.Size.Width, target.Size.Height)
	}
	return b.String()
}

// DeployPath resolves a target's configured output path below deployDir.
func DeployPath(deployDir string, target Target) (string, error) {
	path := target.DeployTemplate
	path = strings.ReplaceAll(path, "{target}", target.ID)
	path = strings.ReplaceAll(path, "{object}", target.ObjectID)
	path = strings.ReplaceAll(path, "{animation}", target.AnimationID)
	path = strings.ReplaceAll(path, "{frame}", target.FrameID)
	for _, variant := range target.Variants {
		path = strings.ReplaceAll(path, "{variant."+variant.AxisID+"}", variant.ValueID)
	}
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

type variantComboValue struct {
	AxisID               string
	ValueID              string
	Description          string
	ReferencePath        string
	ReferenceDescription string
	AxisRefs             []pack.Reference
	ValueRefs            []pack.Reference
}

func variantCombos(variants []pack.Variant) [][]variantComboValue {
	if len(variants) == 0 {
		return [][]variantComboValue{{}}
	}
	combos := [][]variantComboValue{{}}
	for _, variant := range variants {
		var next [][]variantComboValue
		for _, combo := range combos {
			for _, value := range variant.Values {
				item := variantComboValue{AxisID: variant.ID, ValueID: value.ID, Description: value.Description, AxisRefs: variant.References, ValueRefs: value.References}
				if value.Reference != nil {
					item.ReferencePath = value.Reference.Path
					item.ReferenceDescription = value.Reference.Description
				}
				extended := append(append([]variantComboValue{}, combo...), item)
				next = append(next, extended)
			}
		}
		combos = next
	}
	return combos
}
