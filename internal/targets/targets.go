// Package targets expands validated sprite packs into deterministic generation targets.
package targets

import (
	"fmt"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/pack"
)

type Target struct {
	ID             string
	ObjectID       string
	ObjectDesc     string
	AnimationID    string
	AnimationDesc  string
	FrameID        string
	FrameDesc      string
	Size           pack.Size
	Variants       []VariantSelection
	References     []pack.Reference
	DeployTemplate string
	Prompt         string
}

type VariantSelection struct {
	AxisID      string
	ValueID     string
	Description string
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
				out = append(out, makeTarget(p, obj, combo, pack.Animation{}, -1, pack.Frame{}, theme))
				continue
			}
			for _, animation := range obj.Animations {
				for i, frame := range animation.Frames {
					out = append(out, makeTarget(p, obj, combo, animation, i, frame, theme))
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

func makeTarget(p *pack.Pack, obj pack.Object, variants []variantComboValue, animation pack.Animation, frameIndex int, frame pack.Frame, theme string) Target {
	parts := []string{obj.ID}
	refs := append([]pack.Reference{}, p.References...)
	refs = append(refs, obj.References...)
	var selections []VariantSelection
	if animation.ID != "" {
		parts = append(parts, animation.ID)
		refs = append(refs, animation.References...)
	}
	for _, variant := range variants {
		refs = append(refs, variant.AxisRefs...)
		refs = append(refs, variant.ValueRefs...)
		selections = append(selections, VariantSelection{AxisID: variant.AxisID, ValueID: variant.ValueID, Description: variant.Description})
		parts = append(parts, variant.AxisID+"-"+variant.ValueID)
	}
	frameID := ""
	if frameIndex >= 0 {
		frameID = pack.FrameID(frameIndex, frame)
		parts = append(parts, frameID)
		refs = append(refs, frame.References...)
	}
	target := Target{
		ID:             strings.Join(parts, "__"),
		ObjectID:       obj.ID,
		ObjectDesc:     obj.Description,
		AnimationID:    animation.ID,
		AnimationDesc:  animation.Description,
		FrameID:        frameID,
		FrameDesc:      frame.Description,
		Size:           obj.Size,
		Variants:       selections,
		References:     refs,
		DeployTemplate: pack.DeployTemplate(obj),
	}
	target.Prompt = BuildPrompt(theme, target)
	return target
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
	fmt.Fprintf(&b, "Generate one independent sprite image for a final %dx%d target. Do not compose or crop from a sprite sheet.\n", target.Size.Width, target.Size.Height)
	return b.String()
}

type variantComboValue struct {
	AxisID      string
	ValueID     string
	Description string
	AxisRefs    []pack.Reference
	ValueRefs   []pack.Reference
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
				extended := append(append([]variantComboValue{}, combo...), item)
				next = append(next, extended)
			}
		}
		combos = next
	}
	return combos
}
