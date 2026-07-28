package imageio

import (
	"fmt"
	"image"
	"math"
)

func EvaluateCandidate(candidatePath, posePath string, guard int) (Metrics, []string, error) {
	candidate, err := decodeNRGBA(candidatePath)
	if err != nil {
		return Metrics{}, nil, err
	}
	pose, err := decodeNRGBA(posePath)
	if err != nil {
		return Metrics{}, nil, err
	}
	if candidate.Bounds() != pose.Bounds() {
		return Metrics{}, nil, fmt.Errorf("candidate %dx%d and pose %dx%d dimensions differ", candidate.Bounds().Dx(), candidate.Bounds().Dy(), pose.Bounds().Dx(), pose.Bounds().Dy())
	}
	metrics := compareMasks(candidate, pose, guard)
	return metrics, candidateReasons(metrics), nil
}

func compareMasks(candidate, pose *image.NRGBA, guard int) Metrics {
	bounds := candidate.Bounds()
	var intersection, union, candidateArea, poseArea int
	var candidateX, candidateY, poseX, poseY int
	candidateBounds, poseBounds := emptyOccupiedBounds(bounds), emptyOccupiedBounds(bounds)
	candidateBottom, poseBottom := bounds.Min.Y-1, bounds.Min.Y-1
	edgeIntersection, edgeUnion := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			candidateOccupied, poseOccupied := candidate.NRGBAAt(x, y).A > 0, pose.NRGBAAt(x, y).A > 0
			if candidateOccupied {
				candidateArea++
				candidateX += x
				candidateY += y
				candidateBounds.include(x, y)
				candidateBottom = max(candidateBottom, y)
			}
			if poseOccupied {
				poseArea++
				poseX += x
				poseY += y
				poseBounds.include(x, y)
				poseBottom = max(poseBottom, y)
			}
			if candidateOccupied && poseOccupied {
				intersection++
			}
			if candidateOccupied || poseOccupied {
				union++
			}
			candidateEdge, poseEdge := alphaEdge(candidate, x, y), alphaEdge(pose, x, y)
			if candidateEdge && poseEdge {
				edgeIntersection++
			}
			if candidateEdge || poseEdge {
				edgeUnion++
			}
		}
	}
	overlap, edgeAgreement := ratio(intersection, union), ratio(edgeIntersection, edgeUnion)
	areaDelta, boundsDelta, centerDistance := 1.0, 1.0, 1.0
	if candidateArea > 0 && poseArea > 0 {
		areaDelta = math.Abs(float64(candidateArea-poseArea)) / float64(max(candidateArea, poseArea))
		boundsDelta = occupiedBoundsDelta(candidateBounds, poseBounds, bounds)
		candidateCenterX, candidateCenterY := float64(candidateX)/float64(candidateArea), float64(candidateY)/float64(candidateArea)
		poseCenterX, poseCenterY := float64(poseX)/float64(poseArea), float64(poseY)/float64(poseArea)
		centerDistance = math.Hypot(candidateCenterX-poseCenterX, candidateCenterY-poseCenterY) / math.Hypot(float64(bounds.Dx()), float64(bounds.Dy()))
	}
	components, secondaryComponents := foregroundComponents(candidate)
	edgeOccupied := guardOccupied(candidate, guard)
	cellEdgeOccupied := guardOccupied(candidate, 1)
	backdropLike := occupiedBoundsLookLikeBackdrop(candidateArea, candidateBounds, bounds)
	paletteDistance := averagePaletteDistance(candidate, pose)
	baselineDelta := 1.0
	if candidateBottom >= bounds.Min.Y && poseBottom >= bounds.Min.Y {
		baselineDelta = math.Abs(float64(candidateBottom-poseBottom)) / float64(bounds.Dy())
	}
	score := overlap*0.35 + edgeAgreement*0.2 + (1-areaDelta)*0.15 + (1-centerDistance)*0.1 + (1-baselineDelta)*0.1 + (1-paletteDistance)*0.1
	if components != 1 || edgeOccupied {
		score -= 1
	}
	return Metrics{
		SilhouetteOverlap: overlap, EdgeAgreement: edgeAgreement, OccupiedAreaDelta: areaDelta,
		OccupiedBoundsDelta: boundsDelta, CenterDistance: centerDistance, BaselineDelta: baselineDelta,
		PaletteDistance: paletteDistance, Components: components, SecondaryComponents: secondaryComponents,
		EdgeGuardOccupied: edgeOccupied, CellEdgeOccupied: cellEdgeOccupied, BackdropLike: backdropLike, Score: score,
	}
}

func occupiedBoundsLookLikeBackdrop(area int, occupied occupiedBounds, canvas image.Rectangle) bool {
	if area == 0 {
		return false
	}
	width := occupied.maxX - occupied.minX + 1
	height := occupied.maxY - occupied.minY + 1
	if width*5 < canvas.Dx()*3 || height*5 < canvas.Dy()*3 {
		return false
	}
	return area*100 >= width*height*90
}

type occupiedBounds struct {
	minX int
	minY int
	maxX int
	maxY int
}

func emptyOccupiedBounds(canvas image.Rectangle) occupiedBounds {
	return occupiedBounds{minX: canvas.Max.X, minY: canvas.Max.Y, maxX: canvas.Min.X - 1, maxY: canvas.Min.Y - 1}
}

func (b *occupiedBounds) include(x, y int) {
	b.minX = min(b.minX, x)
	b.minY = min(b.minY, y)
	b.maxX = max(b.maxX, x)
	b.maxY = max(b.maxY, y)
}

func occupiedBoundsDelta(candidate, pose occupiedBounds, canvas image.Rectangle) float64 {
	deltaX := max(absInt(candidate.minX-pose.minX), absInt(candidate.maxX-pose.maxX))
	deltaY := max(absInt(candidate.minY-pose.minY), absInt(candidate.maxY-pose.maxY))
	return max(float64(deltaX)/float64(canvas.Dx()), float64(deltaY)/float64(canvas.Dy()))
}

func alphaEdge(img *image.NRGBA, x, y int) bool {
	occupied := img.NRGBAAt(x, y).A > 0
	for _, point := range [...]image.Point{{X: x - 1, Y: y}, {X: x + 1, Y: y}, {X: x, Y: y - 1}, {X: x, Y: y + 1}} {
		if !point.In(img.Bounds()) || (img.NRGBAAt(point.X, point.Y).A > 0) != occupied {
			return true
		}
	}
	return false
}

func foregroundComponents(img *image.NRGBA) (int, int) {
	bounds := img.Bounds()
	seen := make([]bool, bounds.Dx()*bounds.Dy())
	var sizes []int
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			index := (y-bounds.Min.Y)*bounds.Dx() + x - bounds.Min.X
			if seen[index] || img.NRGBAAt(x, y).A == 0 {
				continue
			}
			size := 0
			queue := []image.Point{{X: x, Y: y}}
			seen[index] = true
			for head := 0; head < len(queue); head++ {
				point := queue[head]
				size++
				for _, next := range [...]image.Point{
					{X: point.X - 1, Y: point.Y - 1}, {X: point.X, Y: point.Y - 1}, {X: point.X + 1, Y: point.Y - 1},
					{X: point.X - 1, Y: point.Y}, {X: point.X + 1, Y: point.Y},
					{X: point.X - 1, Y: point.Y + 1}, {X: point.X, Y: point.Y + 1}, {X: point.X + 1, Y: point.Y + 1},
				} {
					if !next.In(bounds) || img.NRGBAAt(next.X, next.Y).A == 0 {
						continue
					}
					nextIndex := (next.Y-bounds.Min.Y)*bounds.Dx() + next.X - bounds.Min.X
					if !seen[nextIndex] {
						seen[nextIndex] = true
						queue = append(queue, next)
					}
				}
			}
			sizes = append(sizes, size)
		}
	}
	largest := 0
	for _, size := range sizes {
		largest = max(largest, size)
	}
	minimumPrimaryArea := max(4, (largest*3+4)/5)
	primary, secondary := 0, 0
	for _, size := range sizes {
		if size >= minimumPrimaryArea {
			primary++
		} else if size >= 4 {
			secondary++
		}
	}
	return primary, secondary
}

func guardOccupied(img *image.NRGBA, guard int) bool {
	if guard <= 0 {
		return false
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if x-bounds.Min.X >= guard && bounds.Max.X-1-x >= guard && y-bounds.Min.Y >= guard && bounds.Max.Y-1-y >= guard {
				continue
			}
			if img.NRGBAAt(x, y).A > 0 {
				return true
			}
		}
	}
	return false
}

func averagePaletteDistance(candidate, pose *image.NRGBA) float64 {
	var distance float64
	count := 0
	for y := candidate.Bounds().Min.Y; y < candidate.Bounds().Max.Y; y++ {
		for x := candidate.Bounds().Min.X; x < candidate.Bounds().Max.X; x++ {
			left, right := candidate.NRGBAAt(x, y), pose.NRGBAAt(x, y)
			if left.A == 0 || right.A == 0 {
				continue
			}
			distance += math.Sqrt(linearDistance(left, PaletteColor{R: right.R, G: right.G, B: right.B}) / 3)
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return min(1, distance/float64(count))
}

func ratio(left, right int) float64 {
	if right == 0 {
		return 0
	}
	return float64(left) / float64(right)
}
