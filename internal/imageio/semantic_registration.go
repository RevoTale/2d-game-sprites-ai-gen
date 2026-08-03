package imageio

import (
	"fmt"
	"image"
	"image/draw"
	"math"
)

// SemanticUnitTransform records the canonical appearance scale and the one
// shared output anchor per direction. Independent provider boards may require
// different source-coordinate scales to reproduce this canonical appearance.
type SemanticUnitTransform struct {
	Scale            float64       `json:"scale"`
	DirectionAnchors []image.Point `json:"directionAnchors"`
}

// SemanticPoseSet is one provider board expressed in its own source-coordinate
// scale. Every pose of one direction must use the same scale.
type SemanticPoseSet struct {
	PosesByDirection [][]SemanticPose
	DirectionScales  []float64
}

// PrepareSemanticPosesForSharedBodyAnchor retains every detected grounded
// body pivot so normalization maps it to the direction's shared output anchor.
// Provider placement inside a logical board slot is layout drift, not motion;
// action extent remains intact because complete pose bounds are preserved.
func PrepareSemanticPosesForSharedBodyAnchor(
	poses []SemanticPose,
	framesPerDirection int,
) ([]SemanticPose, []image.Point, error) {
	if len(poses) == 0 || framesPerDirection <= 0 ||
		len(poses)%framesPerDirection != 0 {
		return nil, nil, fmt.Errorf(
			"frame-zero registration requires complete direction rows",
		)
	}
	registered := append([]SemanticPose(nil), poses...)
	directionCount := len(poses) / framesPerDirection
	offsets := make([]image.Point, directionCount)
	for directionIndex := range directionCount {
		start := directionIndex * framesPerDirection
		calibration := poses[start]
		offsets[directionIndex] = calibration.Pivot.Sub(calibration.Anchor)
	}
	return registered, offsets, nil
}

// ConstrainSemanticUnitAnchors minimally shifts each preferred neutral-view
// anchor into the interval where every complete pose of that direction fits.
// The reference-derived scale and the one-anchor-per-direction invariant remain
// unchanged.
func ConstrainSemanticUnitAnchors(
	transform SemanticUnitTransform,
	posesByDirection [][]SemanticPose,
	width, height int,
) (SemanticUnitTransform, error) {
	scales := make([]float64, len(transform.DirectionAnchors))
	for index := range scales {
		scales[index] = transform.Scale
	}
	return ConstrainSemanticUnitAnchorsAcrossPoseSets(
		transform,
		[]SemanticPoseSet{{
			PosesByDirection: posesByDirection,
			DirectionScales:  scales,
		}},
		width,
		height,
	)
}

// ConstrainSemanticUnitAnchorsAcrossPoseSets intersects the feasible output
// anchor ranges of the canonical master and every independently calibrated
// animation board. It never changes a pose-set scale to make content fit.
func ConstrainSemanticUnitAnchorsAcrossPoseSets(
	transform SemanticUnitTransform,
	poseSets []SemanticPoseSet,
	width, height int,
) (SemanticUnitTransform, error) {
	if transform.Scale <= 0 || width <= 0 || height <= 0 ||
		len(transform.DirectionAnchors) == 0 || len(poseSets) == 0 {
		return SemanticUnitTransform{}, fmt.Errorf(
			"semantic anchor constraint requires a positive transform, frame, and pose sets",
		)
	}
	safe := image.Rect(0, 0, width, height).Inset(
		CanonicalFrameEdgePadding(width, height),
	)
	if safe.Empty() {
		return SemanticUnitTransform{}, fmt.Errorf(
			"%w: production frame has no safe area",
			ErrProductionFrameClipping,
		)
	}
	adjusted := transform
	adjusted.DirectionAnchors = append(
		[]image.Point(nil),
		transform.DirectionAnchors...,
	)
	for directionIndex := range transform.DirectionAnchors {
		minX, maxX := safe.Min.X, safe.Max.X
		minY, maxY := safe.Min.Y, safe.Max.Y
		for setIndex, poseSet := range poseSets {
			if len(poseSet.PosesByDirection) != len(transform.DirectionAnchors) ||
				len(poseSet.DirectionScales) != len(transform.DirectionAnchors) {
				return SemanticUnitTransform{}, fmt.Errorf(
					"semantic pose set %02d has mismatched directions or scales",
					setIndex,
				)
			}
			poses := poseSet.PosesByDirection[directionIndex]
			scale := poseSet.DirectionScales[directionIndex]
			if len(poses) == 0 || scale <= 0 {
				return SemanticUnitTransform{}, fmt.Errorf(
					"semantic pose set %02d direction %02d has no poses or positive scale",
					setIndex,
					directionIndex,
				)
			}
			for _, pose := range poses {
				relative := semanticPoseDestination(
					pose,
					scale,
					image.Point{},
				)
				minX = max(minX, safe.Min.X-relative.Min.X)
				maxX = min(maxX, safe.Max.X-relative.Max.X)
				minY = max(minY, safe.Min.Y-relative.Min.Y)
				maxY = min(maxY, safe.Max.Y-relative.Max.Y)
			}
		}
		if minX > maxX || minY > maxY {
			return SemanticUnitTransform{}, fmt.Errorf(
				"%w: direction %02d has no shared feasible anchor within %v",
				ErrProductionFrameClipping,
				directionIndex,
				safe,
			)
		}
		preferred := transform.DirectionAnchors[directionIndex]
		adjusted.DirectionAnchors[directionIndex] = image.Pt(
			max(minX, min(maxX, preferred.X)),
			max(minY, min(maxY, preferred.Y)),
		)
	}
	return adjusted, nil
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
	framesPerDirection int,
) ([]CanonicalTransform, error) {
	scales := make([]float64, len(transform.DirectionAnchors))
	for index := range scales {
		scales[index] = transform.Scale
	}
	return WriteCalibratedSemanticPoses(
		posePaths,
		poses,
		outputPaths,
		width,
		height,
		palette,
		transform.DirectionAnchors,
		scales,
		framesPerDirection,
	)
}

// WriteCalibratedSemanticPoses applies one source-coordinate scale per
// direction and the unit's already-proven shared output anchors. It cannot
// fit, center, crop, or shrink any pose independently.
func WriteCalibratedSemanticPoses(
	posePaths []string,
	poses []SemanticPose,
	outputPaths []string,
	width, height int,
	palette []PaletteColor,
	directionAnchors []image.Point,
	directionScales []float64,
	framesPerDirection int,
) ([]CanonicalTransform, error) {
	if len(posePaths) == 0 ||
		len(posePaths) != len(poses) ||
		len(posePaths) != len(outputPaths) {
		return nil, fmt.Errorf(
			"semantic normalization requires matching non-empty poses and outputs",
		)
	}
	if framesPerDirection <= 0 ||
		len(directionAnchors) == 0 ||
		len(directionAnchors) != len(directionScales) ||
		len(poses) != len(directionAnchors)*framesPerDirection {
		return nil, fmt.Errorf(
			"semantic normalization has %d poses for %d direction anchors and %d frames per direction",
			len(poses),
			len(directionAnchors),
			framesPerDirection,
		)
	}
	for directionIndex, scale := range directionScales {
		if scale <= 0 {
			return nil, fmt.Errorf(
				"semantic normalization direction %02d has non-positive scale",
				directionIndex,
			)
		}
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
		directionIndex := index / framesPerDirection
		anchor := directionAnchors[directionIndex]
		scale := directionScales[directionIndex]
		destination := semanticPoseDestination(pose, scale, anchor)
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
			Scale: scale, Baseline: anchor.Y,
			CenterX: anchor.X,
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
	scale float64,
	anchor image.Point,
) image.Rectangle {
	left := anchor.X + int(math.Round(
		float64(pose.Bounds.Min.X-pose.Pivot.X)*scale,
	))
	top := anchor.Y + int(math.Round(
		float64(pose.Bounds.Min.Y-pose.Pivot.Y)*scale,
	))
	width := max(1, int(math.Round(float64(pose.Bounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(pose.Bounds.Dy())*scale)))
	return image.Rect(left, top, left+width, top+height)
}
