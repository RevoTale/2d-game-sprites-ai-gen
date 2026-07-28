package imageio

import (
	"errors"
	"fmt"
	"image"
)

// ErrUnsafeCharacterMasterRegistration identifies generated geometry that
// cannot be registered without changing or losing foreground pixels.
var ErrUnsafeCharacterMasterRegistration = errors.New("unsafe character-master registration")

// CellRegistration records the only mechanical correction allowed for a
// generated character master: translating every pixel assigned to one
// direction by one shared offset. Registration never crops or rescales art.
type CellRegistration struct {
	Cell             int             `json:"cell"`
	OffsetX          int             `json:"offsetX"`
	OffsetY          int             `json:"offsetY"`
	SourceBounds     image.Rectangle `json:"sourceBounds"`
	RegisteredBounds image.Rectangle `json:"registeredBounds"`
}

type registrationComponent struct {
	points []image.Point
	bounds image.Rectangle
	cell   int
}

// RegisterCharacterMasterBoard moves complete generated direction groups into
// their deterministic safe cells. Logical cell overflow is recoverable because
// the character master is review evidence, not a deployable frame. Actual
// provider-canvas clipping, ambiguous ownership, missing subjects, merged
// subjects, and groups too large for a safe cell remain hard failures.
func RegisterCharacterMasterBoard(sourcePath, outputPath string, layout GridLayout, inset int) ([]CellRegistration, error) {
	if inset < 1 || inset*2 >= layout.CellWidth || inset*2 >= layout.CellHeight {
		return nil, fmt.Errorf("invalid character-master registration inset %d", inset)
	}
	source, err := decodeNRGBA(sourcePath)
	if err != nil {
		return nil, err
	}
	expected := image.Rect(0, 0, layout.Width(), layout.Height())
	if source.Bounds() != expected {
		return nil, registrationFailure(fmt.Sprintf("character master is %dx%d, expected %dx%d", source.Bounds().Dx(), source.Bounds().Dy(), expected.Dx(), expected.Dy()))
	}
	if guardOccupied(source, 1) {
		return nil, registrationFailure("character master foreground touches the provider canvas edge")
	}
	components, err := registrationComponents(source, layout)
	if err != nil {
		return nil, err
	}
	groups := make([][]registrationComponent, layout.Count)
	for _, component := range components {
		if component.cell < 0 {
			return nil, registrationFailure("character master has foreground not owned by an expected cell")
		}
		groups[component.cell] = append(groups[component.cell], component)
	}

	registered := image.NewNRGBA(expected)
	records := make([]CellRegistration, layout.Count)
	for cell := 0; cell < layout.Count; cell++ {
		group := groups[cell]
		if len(group) == 0 {
			return nil, registrationFailure(fmt.Sprintf("character master cell %02d contains no foreground subject", cell))
		}
		groupImage := image.NewNRGBA(expected)
		groupBounds := image.Rectangle{}
		for _, component := range group {
			if groupBounds.Empty() {
				groupBounds = component.bounds
			} else {
				groupBounds = groupBounds.Union(component.bounds)
			}
			for _, point := range component.points {
				groupImage.SetNRGBA(point.X, point.Y, source.NRGBAAt(point.X, point.Y))
			}
		}
		primary, _ := foregroundComponents(groupImage)
		if primary != 1 {
			return nil, registrationFailure(fmt.Sprintf("character master cell %02d has %d primary foreground subjects", cell, primary))
		}
		safe := layout.Cell(cell).Inset(inset)
		if groupBounds.Dx() > safe.Dx() || groupBounds.Dy() > safe.Dy() {
			return nil, registrationFailure(fmt.Sprintf("character master cell %02d subject %dx%d cannot fit safe cell %dx%d", cell, groupBounds.Dx(), groupBounds.Dy(), safe.Dx(), safe.Dy()))
		}
		offset := minimumTranslationInto(groupBounds, safe)
		registeredBounds := groupBounds.Add(offset)
		for _, component := range group {
			for _, point := range component.points {
				target := point.Add(offset)
				if registered.NRGBAAt(target.X, target.Y).A != 0 {
					return nil, registrationFailure(fmt.Sprintf("character master cell %02d registration overlaps another subject", cell))
				}
				registered.SetNRGBA(target.X, target.Y, source.NRGBAAt(point.X, point.Y))
			}
		}
		records[cell] = CellRegistration{
			Cell: cell, OffsetX: offset.X, OffsetY: offset.Y,
			SourceBounds: groupBounds, RegisteredBounds: registeredBounds,
		}
	}
	if err := writePNG(outputPath, registered); err != nil {
		return nil, err
	}
	return records, nil
}

func registrationComponents(source *image.NRGBA, layout GridLayout) ([]registrationComponent, error) {
	bounds := source.Bounds()
	seen := make([]bool, bounds.Dx()*bounds.Dy())
	var result []registrationComponent
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			index := (y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X
			if seen[index] || source.NRGBAAt(x, y).A == 0 {
				continue
			}
			component := registrationComponent{cell: -1}
			queue := []image.Point{{X: x, Y: y}}
			seen[index] = true
			owned := -1
			for head := 0; head < len(queue); head++ {
				point := queue[head]
				component.points = append(component.points, point)
				pointBounds := image.Rect(point.X, point.Y, point.X+1, point.Y+1)
				if component.bounds.Empty() {
					component.bounds = pointBounds
				} else {
					component.bounds = component.bounds.Union(pointBounds)
				}
				if cell := expectedCellAt(layout, point); cell >= 0 {
					if owned >= 0 && owned != cell {
						return nil, registrationFailure(fmt.Sprintf("character master foreground component spans expected cells %02d and %02d", owned, cell))
					} else {
						owned = cell
						component.cell = cell
					}
				}
				for _, next := range neighboringPixels(point) {
					if !next.In(bounds) || source.NRGBAAt(next.X, next.Y).A == 0 {
						continue
					}
					nextIndex := (next.Y-bounds.Min.Y)*bounds.Dx() + next.X - bounds.Min.X
					if !seen[nextIndex] {
						seen[nextIndex] = true
						queue = append(queue, next)
					}
				}
			}
			result = append(result, component)
		}
	}
	return result, nil
}

func registrationFailure(message string) error {
	return fmt.Errorf("%w: %s", ErrUnsafeCharacterMasterRegistration, message)
}

func neighboringPixels(point image.Point) [8]image.Point {
	return [8]image.Point{
		{X: point.X - 1, Y: point.Y - 1}, {X: point.X, Y: point.Y - 1}, {X: point.X + 1, Y: point.Y - 1},
		{X: point.X - 1, Y: point.Y}, {X: point.X + 1, Y: point.Y},
		{X: point.X - 1, Y: point.Y + 1}, {X: point.X, Y: point.Y + 1}, {X: point.X + 1, Y: point.Y + 1},
	}
}

func minimumTranslationInto(source, destination image.Rectangle) image.Point {
	offset := image.Point{}
	if source.Min.X < destination.Min.X {
		offset.X = destination.Min.X - source.Min.X
	}
	if source.Max.X+offset.X > destination.Max.X {
		offset.X = destination.Max.X - source.Max.X
	}
	if source.Min.Y < destination.Min.Y {
		offset.Y = destination.Min.Y - source.Min.Y
	}
	if source.Max.Y+offset.Y > destination.Max.Y {
		offset.Y = destination.Max.Y - source.Max.Y
	}
	return offset
}
