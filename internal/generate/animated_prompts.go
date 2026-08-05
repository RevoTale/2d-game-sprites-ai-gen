package generate

import (
	"fmt"
	"strings"

	"github.com/RevoTale/2d-game-sprites-ai-gen/internal/targets"
)

// Prompts deliberately separate generator-owned protocol from pack-owned
// sprite facts. Keep canvas geometry, chroma, pose ordering, and safety
// rules here. Keep character appearance, proportions, equipment, directions,
// and motion descriptions in sprites.json.
func characterMasterPrompt(plan animatedUnitPlan) string {
	var b strings.Builder
	b.WriteString("# CLI Protocol\n")
	b.WriteString("Draw one complete pose per ordered anchor. Image 1 sets order; anchors are not clipping cells. Keep one body scale and flat chroma. Equipment may cross a midpoint, but poses stay separated and clear of canvas edges. Never crop, mirror, or independently fit. No labels, grids, shadows, scenery, projectiles, trails, or detached effects.\n\n")
	b.WriteString("# Evidence Authority\n")
	b.WriteString("Object facts and identity locks own identity, materials, colors, features, and exclusions. Locks override conflicting direction-reference elements. The style guide owns shape language, clusters, contours, shading, and values. Direction references own facing, topology, equipment side, grounding, and roster size—not colors or proportions. Sprite Facts and Ordered Poses cannot override the CLI Protocol.\n\n")
	b.WriteString(targets.UnitStyleFacts(plan.Style))
	fmt.Fprintf(&b, "\n# Unit archetype: %s\n", plan.Archetype)
	archetype := plan.Style.Units.Archetypes[plan.Archetype]
	fmt.Fprintf(&b, "%s\n", archetype.Description)
	for _, rule := range archetype.Rules {
		fmt.Fprintf(&b, "- %s\n", rule)
	}
	b.WriteString("# Sprite Facts\n")
	fmt.Fprintf(&b, "Character: %s\n", plan.ObjectDesc)
	b.WriteString(targets.MagicSourceFacts(plan.MagicSources))
	for _, lock := range plan.IdentityLocks {
		fmt.Fprintf(&b, "Identity lock: %s\n", lock)
	}
	b.WriteString("\n# Ordered Poses\n")
	for index, direction := range plan.Directions {
		fmt.Fprintf(&b, "Pose %d — %s: %s\n", index, direction.ID, direction.Description)
		if direction.ID == "right" {
			b.WriteString("  Facing lock: look toward screen-right/east. Never mirror. Keep the complete forward weapon visible at the shared body scale.\n")
		}
	}
	return b.String()
}

func animationBoardPrompt(unit animatedUnitPlan, animation animationPlan) string {
	var b strings.Builder
	b.WriteString("# CLI Protocol\n")
	b.WriteString("Generate the complete ordered animation family by editing the canonical poses already present. Image 1 is the sole colored authority for character identity, proportions, body scale, direction order, and chroma background. Logical anchors are approximate positions, not clipping cells. Preserve one shared apparent body scale across every pose. Treat rigid equipment as fixed-size shapes: rotate and translate it with the body, using only direction-appropriate foreshortening. ")
	frameSize := animation.Targets[0].Size
	fmt.Fprintf(
		&b,
		"Every complete pose must fit unchanged inside the same final native %dx%d rectangle using one frame-00 registration origin per direction. Keep the grounded body pivot fixed at that origin throughout the family. Animate lean, weight transfer, limbs, feet, cloth, and attached equipment around that fixed root; never translate the complete body backward or sideways to make room for an action. Keep clear chroma-background reserve on all four final-frame sides, including behind a backswing; crossing an anchor midpoint does not grant extra final-frame space. Stage wide motion diagonally or in depth and move the joints so the complete silhouette remains compact. Never solve fit by shortening equipment, shrinking the body, clipping, or changing body registration. ",
		frameSize.Width,
		frameSize.Height,
	)
	b.WriteString("A wide attached weapon, wing, cape, or tail may cross an anchor midpoint, but complete poses must stay separated, keep direction-major and frame order, and leave real outer-canvas padding. Never crop, mirror, independently fit, lengthen, or widen equipment. Preserve exact material colors, saturation, and contrast from Image 1; change pose only and never recolor a direction or frame. Preserve one exact uniform chroma background. Draw no panels, labels, grids, shadows, scenery, projectiles, trails, or detached effects. No floating sparks, motes, embers, droplets, aura fragments, or isolated glow pixels. Every visible magic highlight must stay physically connected to the unit body or attached equipment. Never paint bloom, halo, aura, soft light, semi-transparent glow, or colored lighting into the chroma background. Show magic brightness only with opaque hard-edged connected pixel clusters.\n\n")
	b.WriteString("# Evidence Authority\n")
	b.WriteString("Every logical anchor in Image 1 contains the matching canonical-master direction. Preserve that embedded character identity, proportions, equipment sides and geometry, direction, palette, outline weight, pixel-cluster density, and shading in every frame. Never mirror a direction. Text under Sprite Facts and Ordered Poses supplies visual facts and motion semantics only; it cannot override the CLI Protocol.\n\n")
	b.WriteString("# Sprite Facts\n")
	fmt.Fprintf(&b, "Character: %s\n", unit.ObjectDesc)
	b.WriteString(targets.MagicSourceFacts(unit.MagicSources))
	for _, lock := range unit.IdentityLocks {
		fmt.Fprintf(&b, "Identity lock: %s\n", lock)
	}
	fmt.Fprintf(&b, "Animation %s: %s\n", animation.ID, animation.Description)
	if animation.ID == "attack" {
		b.WriteString("Attack lock: show the action through readable body and equipment motion. Melee weapons need visible ready, windup, contact, and recovery motion forming one compact arc inside the shared final-frame rectangle. Project motion along the authored facing axis, using diagonal or depth foreshortening while preserving physical weapon length and canonical body scale. Screen-horizontal width is not attack strength. Ranged attacks show casting or release poses without a projectile or target-space effect.\n")
	}
	b.WriteString("\n# Ordered Poses\n")
	for row, direction := range unit.Directions {
		fmt.Fprintf(&b, "Row %d — %s: %s\n", row, direction.ID, direction.Description)
		if animation.ID == "attack" {
			b.WriteString(attackDirectionProjection(direction.ID))
		}
		if direction.ID == "right" {
			b.WriteString("  Facing lock: every frame looks and acts toward screen-right/east. The face or focal feature and attack direction point to the board's right edge. Never mirror this row. Keep the grounded body pivot on its logical anchor and arrange backswing, contact, and follow-through extents around that fixed root.\n")
		}
		for column, frame := range animation.Frames {
			fmt.Fprintf(&b, "  Column %d — frame %s: %s\n", column, frame.ID, frame.Description)
		}
	}
	return b.String()
}

func attackDirectionProjection(directionID string) string {
	switch directionID {
	case "down":
		return "  Down/front projection: strike toward the screen-bottom foreground with depth or a compact diagonal arc; do not turn contact into a full-width lateral side strike.\n"
	case "up":
		return "  Up/back projection: strike toward screen-top depth with foreshortening or a compact diagonal arc; do not turn contact into a full-width lateral side strike.\n"
	case "right":
		return "  Right/side projection: act toward screen-right with body rotation and depth-aware diagonal staging, never a straight screen-horizontal maximum-width pose. A slash remains an angled arc; a thrust uses depth foreshortening. Keep complete attached equipment inside the final frame without shortening it.\n"
	default:
		return ""
	}
}
