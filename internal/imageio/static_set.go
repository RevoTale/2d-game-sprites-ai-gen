package imageio

import (
	"fmt"
	"image"
	"image/draw"
	"os"
)

// StaticSetScaleCalibrationVersion identifies the deterministic shared-scale
// transform used for coupled static-set parts.
const StaticSetScaleCalibrationVersion = 1

// StaticSetPart declares one recovered static-set crop and its production
// canvas. Every part in one call receives the same scale.
type StaticSetPart struct {
	ID           string
	SourcePath   string
	OutputPath   string
	Size         image.Point
	Registration SubjectRegistrationMode
}

// StaticSetScaleCalibration records the one limiting transform selected for a
// complete coupled set. An empty limiter means every source already fit at 1x.
type StaticSetScaleCalibration struct {
	Version        int     `json:"version"`
	Scale          float64 `json:"scale"`
	Numerator      int     `json:"numerator"`
	Denominator    int     `json:"denominator"`
	LimitingPartID string  `json:"limitingPartId,omitempty"`
	LimitingAxis   string  `json:"limitingAxis,omitempty"`
}

type measuredStaticSetPart struct {
	spec        StaticSetPart
	foreground  *image.NRGBA
	bounds      image.Rectangle
	safe        image.Rectangle
	destination image.Rectangle
}

// WriteSharedScaleTransparentStaticSet measures every recovered crop before
// writing any output, selects one non-upscaling transform, and applies that
// transform to the complete set. Per-part fitting is intentionally forbidden.
func WriteSharedScaleTransparentStaticSet(
	parts []StaticSetPart,
	locked []PaletteColor,
) (StaticSetScaleCalibration, error) {
	calibration := StaticSetScaleCalibration{
		Version:     StaticSetScaleCalibrationVersion,
		Scale:       1,
		Numerator:   1,
		Denominator: 1,
	}
	if len(parts) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("static set requires at least one part")
	}
	if len(locked) == 0 {
		return StaticSetScaleCalibration{}, fmt.Errorf("static set requires a shared locked palette")
	}
	measured := make([]measuredStaticSetPart, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item, err := measureStaticSetPart(part)
		if err != nil {
			return StaticSetScaleCalibration{}, err
		}
		if _, exists := seen[part.ID]; exists {
			return StaticSetScaleCalibration{}, fmt.Errorf("duplicate static set part id %q", part.ID)
		}
		seen[part.ID] = struct{}{}
		measured = append(measured, item)
		considerStaticSetLimit(
			&calibration,
			part.ID,
			"width",
			item.safe.Dx(),
			item.bounds.Dx(),
		)
		considerStaticSetLimit(
			&calibration,
			part.ID,
			"height",
			item.safe.Dy(),
			item.bounds.Dy(),
		)
	}
	calibration.Scale = float64(calibration.Numerator) / float64(calibration.Denominator)
	for index := range measured {
		destination, err := staticSetDestination(measured[index], calibration)
		if err != nil {
			return StaticSetScaleCalibration{}, err
		}
		measured[index].destination = destination
	}
	if err := writeMeasuredStaticSet(measured, locked); err != nil {
		return StaticSetScaleCalibration{}, err
	}
	return calibration, nil
}

func measureStaticSetPart(part StaticSetPart) (measuredStaticSetPart, error) {
	if part.ID == "" {
		return measuredStaticSetPart{}, fmt.Errorf("static set part id is required")
	}
	if part.SourcePath == "" || part.OutputPath == "" {
		return measuredStaticSetPart{}, fmt.Errorf("static set part %q requires source and output paths", part.ID)
	}
	if part.Size.X <= 0 || part.Size.Y <= 0 {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q target size must be positive, got %dx%d",
			part.ID,
			part.Size.X,
			part.Size.Y,
		)
	}
	if part.Registration != SubjectRegistrationCentered &&
		part.Registration != SubjectRegistrationGrounded {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q has unsupported registration %q",
			part.ID,
			part.Registration,
		)
	}
	decoded, err := decodeNRGBA(part.SourcePath)
	if err != nil {
		return measuredStaticSetPart{}, fmt.Errorf("decode static set part %q: %w", part.ID, err)
	}
	foreground := image.NewNRGBA(decoded.Bounds())
	draw.Draw(foreground, foreground.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	bounds, err := alphaBounds(foreground)
	if err != nil {
		return measuredStaticSetPart{}, fmt.Errorf("static set part %q foreground: %w", part.ID, err)
	}
	guard := max(1, min(part.Size.X, part.Size.Y)/32)
	safe := image.Rect(guard, guard, part.Size.X-guard, part.Size.Y-guard)
	if safe.Empty() {
		return measuredStaticSetPart{}, fmt.Errorf(
			"static set part %q target %dx%d has no safe foreground rectangle",
			part.ID,
			part.Size.X,
			part.Size.Y,
		)
	}
	return measuredStaticSetPart{
		spec:       part,
		foreground: foreground,
		bounds:     bounds,
		safe:       safe,
	}, nil
}

func considerStaticSetLimit(
	calibration *StaticSetScaleCalibration,
	partID, axis string,
	numerator, denominator int,
) {
	if numerator >= denominator {
		return
	}
	left := int64(numerator) * int64(calibration.Denominator)
	right := int64(calibration.Numerator) * int64(denominator)
	if left >= right {
		return
	}
	calibration.Numerator = numerator
	calibration.Denominator = denominator
	calibration.LimitingPartID = partID
	calibration.LimitingAxis = axis
}

func staticSetDestination(
	part measuredStaticSetPart,
	calibration StaticSetScaleCalibration,
) (image.Rectangle, error) {
	width := int(int64(part.bounds.Dx()) * int64(calibration.Numerator) / int64(calibration.Denominator))
	height := int(int64(part.bounds.Dy()) * int64(calibration.Numerator) / int64(calibration.Denominator))
	if width <= 0 || height <= 0 {
		return image.Rectangle{}, fmt.Errorf(
			"static set part %q becomes empty at shared scale %.6f",
			part.spec.ID,
			calibration.Scale,
		)
	}
	left := part.safe.Min.X + (part.safe.Dx()-width)/2
	top := part.safe.Min.Y + (part.safe.Dy()-height)/2
	if part.spec.Registration == SubjectRegistrationGrounded {
		top = part.safe.Max.Y - height
	}
	destination := image.Rect(left, top, left+width, top+height)
	if !destination.In(part.safe) {
		return image.Rectangle{}, fmt.Errorf(
			"static set part %q destination %v exceeds safe target %v",
			part.spec.ID,
			destination,
			part.safe,
		)
	}
	return destination, nil
}

func writeMeasuredStaticSet(parts []measuredStaticSetPart, locked []PaletteColor) error {
	temporaryPaths := make([]string, 0, len(parts))
	for _, part := range parts {
		temporaryPath := part.spec.OutputPath + ".static-set.tmp"
		normalized := image.NewNRGBA(image.Rectangle{Max: part.spec.Size})
		areaScale(normalized, part.destination, part.foreground, part.bounds)
		normalized = applyPalette(normalized, locked)
		if err := writePNG(temporaryPath, normalized); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths)
			_ = os.Remove(temporaryPath)
			return fmt.Errorf("write static set part %q: %w", part.spec.ID, err)
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
	}
	for index, part := range parts {
		if err := os.Rename(temporaryPaths[index], part.spec.OutputPath); err != nil {
			removeStaticSetTemporaryPaths(temporaryPaths[index:])
			return fmt.Errorf("publish static set part %q: %w", part.spec.ID, err)
		}
	}
	return nil
}

func removeStaticSetTemporaryPaths(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
