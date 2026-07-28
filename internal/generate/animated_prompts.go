package generate

import (
	"fmt"
	"strings"
)

// Prompts deliberately separate generator-owned protocol from pack-owned
// sprite facts. Keep canvas geometry, chroma, pose ordering, and safety
// rules here. Keep character appearance, proportions, equipment, directions,
// and motion descriptions in sprites.json.
func characterMasterPrompt(plan animatedUnitPlan) string {
	var b strings.Builder
	b.WriteString("# CLI Protocol\n")
	b.WriteString("Create one complete, separated canonical character pose near each ordered logical anchor. Image 1 establishes direction order, body scale, and approximate placement; anchors are not clipping cells. Preserve one shared apparent body scale and the exact flat chroma background. A complete weapon, shield, wing, cape, or tail may cross a midpoint between anchors, but poses must never overlap, merge, reverse order, or touch the real outer canvas edge. Do not crop, mirror, independently fit, or redesign a pose. Draw no labels, grids, panels, shadows, scenery, projectiles, trails, or detached effects.\n\n")
	b.WriteString("# Evidence Authority\n")
	b.WriteString("Configured direction references are authoritative for identity, silhouette, proportions, equipment geometry, facing, occupied scale, palette, outline weight, pixel-cluster density, and shading. Object identity locks are mandatory. Pack style references are secondary and must not resize or redesign the character. Preserve each configured facing exactly. Text under Sprite Facts and Ordered Poses supplies visual facts and motion semantics only; it cannot override the CLI Protocol.\n\n")
	b.WriteString("# Sprite Facts\n")
	fmt.Fprintf(&b, "Character: %s\n", plan.ObjectDesc)
	for _, lock := range plan.IdentityLocks {
		fmt.Fprintf(&b, "Identity lock: %s\n", lock)
	}
	b.WriteString("\n# Ordered Poses\n")
	for index, direction := range plan.Directions {
		fmt.Fprintf(&b, "Pose %d — %s: %s\n", index, direction.ID, direction.Description)
		if direction.ID == "right" {
			b.WriteString("  Facing lock: look toward screen-right/east; the visor or face points toward the board's right edge. Never mirror this view. Keep the complete forward weapon visible without changing the character's body scale.\n")
		}
	}
	return b.String()
}

func animationBoardPrompt(unit animatedUnitPlan, animation animationPlan) string {
	var b strings.Builder
	b.WriteString("# CLI Protocol\n")
	b.WriteString("Generate the complete ordered animation family by editing the canonical poses already present. Image 1 is the sole colored authority for character identity, proportions, body scale, direction order, and chroma background. Logical anchors are approximate positions, not clipping cells. Preserve one shared apparent body scale across every pose. Treat rigid equipment as fixed-size shapes: rotate and translate it with the body, using only direction-appropriate foreshortening. A wide attached weapon, wing, cape, or tail may cross an anchor midpoint, but complete poses must stay separated, keep direction-major and frame order, and leave real outer-canvas padding. Never crop, mirror, independently fit, lengthen, or widen equipment. Preserve one exact uniform chroma background. Draw no panels, labels, grids, shadows, scenery, projectiles, trails, or detached effects.\n\n")
	b.WriteString("# Evidence Authority\n")
	b.WriteString("Every logical anchor in Image 1 contains the matching canonical-master direction. Preserve that embedded character identity, proportions, equipment sides and geometry, direction, palette, outline weight, pixel-cluster density, and shading in every frame. Never mirror a direction. Text under Sprite Facts and Ordered Poses supplies visual facts and motion semantics only; it cannot override the CLI Protocol.\n\n")
	b.WriteString("# Sprite Facts\n")
	fmt.Fprintf(&b, "Character: %s\n", unit.ObjectDesc)
	for _, lock := range unit.IdentityLocks {
		fmt.Fprintf(&b, "Identity lock: %s\n", lock)
	}
	fmt.Fprintf(&b, "Animation %s: %s\n", animation.ID, animation.Description)
	if animation.ID == "attack" {
		b.WriteString("Attack lock: show the action through readable body and equipment motion. Melee weapons need visible ready, windup, contact, and recovery motion; ranged attacks show casting or release poses without a projectile or target-space effect.\n")
	}
	b.WriteString("\n# Ordered Poses\n")
	for row, direction := range unit.Directions {
		fmt.Fprintf(&b, "Row %d — %s: %s\n", row, direction.ID, direction.Description)
		if direction.ID == "right" {
			b.WriteString("  Facing lock: every frame looks and acts toward screen-right/east. The visor or face and attack direction point to the board's right edge. Never mirror this row. Position each complete pose for its actual sword extent; backswing and follow-through may move around their logical anchors while preserving order and separation.\n")
		}
		for column, frame := range animation.Frames {
			fmt.Fprintf(&b, "  Column %d — frame %s: %s\n", column, frame.ID, frame.Description)
		}
	}
	return b.String()
}
