package imageio

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"sort"
)

// SemanticUnitTransform is the one scale and body pivot shared by every
// generated frame in an animated unit.
type SemanticUnitTransform struct {
	Scale    float64 `json:"scale"`
	Baseline int     `json:"baseline"`
	CenterX  int     `json:"centerX"`
}

// FitSemanticUnitTransform derives one target body scale, then reduces that
// single unit-wide scale only when the complete pose envelope requires it.
func FitSemanticUnitTransform(
	poses []SemanticPose,
	width, height int,
) (SemanticUnitTransform, error) {
	if len(poses) == 0 || width <= 0 || height <= 0 {
		return SemanticUnitTransform{}, fmt.Errorf(
			"semantic unit transform requires poses and a positive frame size",
		)
	}
	coreHeights := make([]int, len(poses))
	for index, pose := range poses {
		if pose.Bounds.Empty() || pose.CoreBounds.Empty() ||
			!pose.Pivot.In(pose.CoreBounds.Inset(-1)) {
			return SemanticUnitTransform{}, fmt.Errorf(
				"pose %02d has invalid body registration evidence",
				index,
			)
		}
		coreHeights[index] = pose.CoreBounds.Dy()
	}
	sort.Ints(coreHeights)
	medianCoreHeight := coreHeights[len(coreHeights)/2]
	targetBodyHeight := max(1, int(math.Round(float64(height)*0.625)))
	transform := SemanticUnitTransform{
		Scale:    float64(targetBodyHeight) / float64(medianCoreHeight),
		Baseline: height - max(2, height/20),
		CenterX:  width / 2,
	}
	safe := image.Rect(0, 0, width, height).Inset(
		CanonicalFrameEdgePadding(width, height),
	)
	transform.Scale = fitSemanticUnitScale(poses, transform, safe)
	for index, pose := range poses {
		if destination := semanticPoseDestination(pose, transform); !destination.In(safe) {
			return SemanticUnitTransform{}, fmt.Errorf(
				"%w: pose %02d extent %v cannot fit shared unit transform in %v",
				ErrProductionFrameClipping,
				index,
				destination,
				safe,
			)
		}
	}
	return transform, nil
}

func fitSemanticUnitScale(
	poses []SemanticPose,
	transform SemanticUnitTransform,
	safe image.Rectangle,
) float64 {
	fits := func(scale float64) bool {
		candidate := transform
		candidate.Scale = scale
		for _, pose := range poses {
			if !semanticPoseDestination(pose, candidate).In(safe) {
				return false
			}
		}
		return true
	}
	if fits(transform.Scale) {
		return transform.Scale
	}

	low, high := 0.0, transform.Scale
	for range 64 {
		middle := (low + high) / 2
		if fits(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	return low
}

// WriteRegisteredSemanticPoses applies an already-proven unit transform. It
// cannot fit, center, crop, or shrink any pose independently.
func WriteRegisteredSemanticPoses(
	posePaths []string,
	poses []SemanticPose,
	outputPaths []string,
	width, height int,
	palette []PaletteColor,
	transform SemanticUnitTransform,
) ([]CanonicalTransform, error) {
	if len(posePaths) == 0 ||
		len(posePaths) != len(poses) ||
		len(posePaths) != len(outputPaths) {
		return nil, fmt.Errorf(
			"semantic normalization requires matching non-empty poses and outputs",
		)
	}
	safe := image.Rect(0, 0, width, height).Inset(
		CanonicalFrameEdgePadding(width, height),
	)
	frames := make([]*image.NRGBA, len(poses))
	evidence := make([]CanonicalTransform, len(poses))
	for index, pose := range poses {
		source, err := decodeNRGBA(posePaths[index])
		if err != nil {
			return nil, err
		}
		if source.Bounds().Size() != pose.Bounds.Size() {
			return nil, fmt.Errorf(
				"pose %02d image is %v, expected recovered extent %v",
				index,
				source.Bounds().Size(),
				pose.Bounds.Size(),
			)
		}
		destination := semanticPoseDestination(pose, transform)
		if !destination.In(safe) {
			return nil, fmt.Errorf(
				"%w: pose %02d cannot fit shared unit transform",
				ErrProductionFrameClipping,
				index,
			)
		}
		frame := image.NewNRGBA(image.Rect(0, 0, width, height))
		areaScale(frame, destination, source, source.Bounds())
		frames[index] = frame
		evidence[index] = CanonicalTransform{
			Scale: transform.Scale, Baseline: transform.Baseline,
			CenterX: transform.CenterX,
			OffsetX: destination.Min.X, OffsetY: destination.Min.Y,
		}
	}
	lockedPalette := append([]PaletteColor(nil), palette...)
	if len(lockedPalette) == 0 {
		evidenceSheet := image.NewNRGBA(
			image.Rect(0, 0, width, height*len(frames)),
		)
		for index, frame := range frames {
			draw.Draw(
				evidenceSheet,
				image.Rect(0, index*height, width, (index+1)*height),
				frame,
				frame.Bounds().Min,
				draw.Src,
			)
		}
		lockedPalette = extractPalette(evidenceSheet, defaultPaletteSize)
	}
	for index, frame := range frames {
		frame = applyPalette(frame, lockedPalette)
		if err := writePNG(outputPaths[index], frame); err != nil {
			return nil, err
		}
		if err := ValidateCanonicalFrame(outputPaths[index], width, height); err != nil {
			return nil, fmt.Errorf("pose %02d: %w", index, err)
		}
	}
	return evidence, nil
}

func semanticPoseDestination(
	pose SemanticPose,
	transform SemanticUnitTransform,
) image.Rectangle {
	left := transform.CenterX + int(math.Round(
		float64(pose.Bounds.Min.X-pose.Pivot.X)*transform.Scale,
	))
	top := transform.Baseline + int(math.Round(
		float64(pose.Bounds.Min.Y-pose.Pivot.Y)*transform.Scale,
	))
	width := max(1, int(math.Round(float64(pose.Bounds.Dx())*transform.Scale)))
	height := max(1, int(math.Round(float64(pose.Bounds.Dy())*transform.Scale)))
	return image.Rect(left, top, left+width, top+height)
}
