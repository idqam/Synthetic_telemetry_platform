package engine

import (
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
)

type Region struct {
	ID        RegionID
	Shape     Polygon
	Biomes    []BiomeShare
	Capacity  int
	Resources Resources
}

type BiomeShare struct {
	BiomeID  BiomeID
	Fraction float64
}

func NewRegion(shape Polygon, capacity int, biomes []BiomeShare) (*Region, error) {
	region := &Region{
		ID:        RegionID(uuid.NewString()),
		Shape:     shape,
		Biomes:    append([]BiomeShare(nil), biomes...),
		Capacity:  capacity,
		Resources: make(Resources),
	}
	if err := region.Validate(); err != nil {
		return nil, err
	}
	return region, nil
}

func (r *Region) Validate() error {
	if r == nil {
		return errors.New("region cannot be nil")
	}
	if r.ID == "" {
		return errors.New("region ID cannot be empty")
	}
	if err := r.Shape.Validate(); err != nil {
		return fmt.Errorf("invalid shape: %w", err)
	}
	if r.Capacity <= 0 {
		return errors.New("capacity must be positive")
	}
	if len(r.Biomes) == 0 {
		return errors.New("region requires at least one biome")
	}

	var total float64
	seen := make(map[BiomeID]struct{}, len(r.Biomes))
	for _, share := range r.Biomes {
		if share.BiomeID == "" {
			return errors.New("biome ID cannot be empty")
		}
		if share.Fraction <= 0 || share.Fraction > 1 {
			return fmt.Errorf("biome %q fraction must be in (0, 1]", share.BiomeID)
		}
		if _, exists := seen[share.BiomeID]; exists {
			return fmt.Errorf("duplicate biome %q in region", share.BiomeID)
		}
		seen[share.BiomeID] = struct{}{}
		total += share.Fraction
	}
	if math.Abs(total-1) > epsilon {
		return fmt.Errorf("biome fractions must sum to 1.0; got %.6f", total)
	}
	if err := r.Resources.Validate(); err != nil {
		return err
	}
	return nil
}

func (r *Region) DominantBiome() BiomeID {
	var dominant BiomeID
	var largest float64
	for _, share := range r.Biomes {
		if share.Fraction > largest {
			dominant = share.BiomeID
			largest = share.Fraction
		}
	}
	return dominant
}

func (r *Region) HasCapacity(entityCount int) bool {
	return entityCount >= 0 && entityCount < r.Capacity
}

func (r *Region) AtCapacity(entityCount int) bool {
	return entityCount >= r.Capacity
}

func (r *Region) OccupancyRatio(entityCount int) float64 {
	if r.Capacity <= 0 {
		return 0
	}
	return clamp(float64(entityCount)/float64(r.Capacity), 0, 1)
}

type Point struct {
	X float64
	Y float64
}

type Bounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func (b Bounds) Width() float64  { return b.MaxX - b.MinX }
func (b Bounds) Height() float64 { return b.MaxY - b.MinY }

func (b Bounds) ContainsBounds(other Bounds) bool {
	return other.MinX >= b.MinX-epsilon &&
		other.MinY >= b.MinY-epsilon &&
		other.MaxX <= b.MaxX+epsilon &&
		other.MaxY <= b.MaxY+epsilon
}

func (b Bounds) overlapsArea(other Bounds) bool {
	return b.MinX < other.MaxX-epsilon &&
		b.MaxX > other.MinX+epsilon &&
		b.MinY < other.MaxY-epsilon &&
		b.MaxY > other.MinY+epsilon
}

type Polygon struct {
	Vertices []Point
}

func NewPolygon(vertices []Point) (Polygon, error) {
	polygon := Polygon{Vertices: append([]Point(nil), vertices...)}
	if err := polygon.Validate(); err != nil {
		return Polygon{}, err
	}
	return polygon, nil
}

func NewRectangle(minX, minY, maxX, maxY float64) (Polygon, error) {
	if maxX <= minX || maxY <= minY {
		return Polygon{}, errors.New("rectangle max coordinates must exceed min coordinates")
	}
	return NewPolygon([]Point{
		{X: minX, Y: minY},
		{X: maxX, Y: minY},
		{X: maxX, Y: maxY},
		{X: minX, Y: maxY},
	})
}

func (p Polygon) Validate() error {
	if len(p.Vertices) < 3 {
		return errors.New("polygon requires at least three vertices")
	}
	for i, vertex := range p.Vertices {
		if math.IsNaN(vertex.X) || math.IsNaN(vertex.Y) ||
			math.IsInf(vertex.X, 0) || math.IsInf(vertex.Y, 0) {
			return fmt.Errorf("vertex %d must contain finite coordinates", i)
		}
	}
	if p.Area() <= epsilon {
		return errors.New("polygon area must be positive")
	}
	if p.selfIntersects() {
		return errors.New("polygon cannot self-intersect")
	}
	return nil
}

func (p Polygon) Bounds() Bounds {
	bounds := Bounds{
		MinX: p.Vertices[0].X,
		MinY: p.Vertices[0].Y,
		MaxX: p.Vertices[0].X,
		MaxY: p.Vertices[0].Y,
	}
	for _, vertex := range p.Vertices[1:] {
		bounds.MinX = math.Min(bounds.MinX, vertex.X)
		bounds.MinY = math.Min(bounds.MinY, vertex.Y)
		bounds.MaxX = math.Max(bounds.MaxX, vertex.X)
		bounds.MaxY = math.Max(bounds.MaxY, vertex.Y)
	}
	return bounds
}

func (p Polygon) Area() float64 {
	var twiceArea float64
	for i := range p.Vertices {
		j := (i + 1) % len(p.Vertices)
		twiceArea += p.Vertices[i].X*p.Vertices[j].Y - p.Vertices[j].X*p.Vertices[i].Y
	}
	return math.Abs(twiceArea) / 2
}

func (p Polygon) Contains(point Point) bool {
	if p.pointOnBoundary(point) {
		return true
	}
	return p.containsStrict(point)
}

func (p Polygon) OverlapsArea(other Polygon) bool {
	if !p.Bounds().overlapsArea(other.Bounds()) {
		return false
	}
	if polygonsEqual(p, other) {
		return true
	}

	for i := range p.Vertices {
		a1 := p.Vertices[i]
		a2 := p.Vertices[(i+1)%len(p.Vertices)]
		for j := range other.Vertices {
			b1 := other.Vertices[j]
			b2 := other.Vertices[(j+1)%len(other.Vertices)]
			if segmentsProperlyIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	for _, vertex := range p.Vertices {
		if other.containsStrict(vertex) {
			return true
		}
	}
	for _, vertex := range other.Vertices {
		if p.containsStrict(vertex) {
			return true
		}
	}
	return p.containsStrict(p.centroid()) && other.containsStrict(p.centroid()) ||
		other.containsStrict(other.centroid()) && p.containsStrict(other.centroid())
}

func (p Polygon) containsStrict(point Point) bool {
	inside := false
	for i, j := 0, len(p.Vertices)-1; i < len(p.Vertices); j, i = i, i+1 {
		pi := p.Vertices[i]
		pj := p.Vertices[j]
		intersects := (pi.Y > point.Y) != (pj.Y > point.Y) &&
			point.X < (pj.X-pi.X)*(point.Y-pi.Y)/(pj.Y-pi.Y)+pi.X
		if intersects {
			inside = !inside
		}
	}
	return inside && !p.pointOnBoundary(point)
}

func (p Polygon) pointOnBoundary(point Point) bool {
	for i := range p.Vertices {
		if pointOnSegment(point, p.Vertices[i], p.Vertices[(i+1)%len(p.Vertices)]) {
			return true
		}
	}
	return false
}

func (p Polygon) centroid() Point {
	var x, y, factor, signedArea float64
	for i := range p.Vertices {
		j := (i + 1) % len(p.Vertices)
		factor = p.Vertices[i].X*p.Vertices[j].Y - p.Vertices[j].X*p.Vertices[i].Y
		x += (p.Vertices[i].X + p.Vertices[j].X) * factor
		y += (p.Vertices[i].Y + p.Vertices[j].Y) * factor
		signedArea += factor
	}
	if math.Abs(signedArea) <= epsilon {
		return p.Vertices[0]
	}
	return Point{X: x / (3 * signedArea), Y: y / (3 * signedArea)}
}

func (p Polygon) selfIntersects() bool {
	n := len(p.Vertices)
	for i := 0; i < n; i++ {
		a1 := p.Vertices[i]
		a2 := p.Vertices[(i+1)%n]
		for j := i + 1; j < n; j++ {
			if i == j || (i+1)%n == j || i == (j+1)%n {
				continue
			}
			b1 := p.Vertices[j]
			b2 := p.Vertices[(j+1)%n]
			if segmentsIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func polygonsEqual(a, b Polygon) bool {
	if len(a.Vertices) != len(b.Vertices) {
		return false
	}
	for start := range b.Vertices {
		if !pointsEqual(a.Vertices[0], b.Vertices[start]) {
			continue
		}
		forward := true
		reverse := true
		for i := range a.Vertices {
			if !pointsEqual(a.Vertices[i], b.Vertices[(start+i)%len(b.Vertices)]) {
				forward = false
			}
			reverseIndex := (start - i + len(b.Vertices)) % len(b.Vertices)
			if !pointsEqual(a.Vertices[i], b.Vertices[reverseIndex]) {
				reverse = false
			}
		}
		if forward || reverse {
			return true
		}
	}
	return false
}

func pointsEqual(a, b Point) bool {
	return math.Abs(a.X-b.X) <= epsilon && math.Abs(a.Y-b.Y) <= epsilon
}

func orientation(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

func pointOnSegment(p, a, b Point) bool {
	return math.Abs(orientation(a, b, p)) <= epsilon &&
		p.X >= math.Min(a.X, b.X)-epsilon && p.X <= math.Max(a.X, b.X)+epsilon &&
		p.Y >= math.Min(a.Y, b.Y)-epsilon && p.Y <= math.Max(a.Y, b.Y)+epsilon
}

func segmentsProperlyIntersect(a, b, c, d Point) bool {
	o1 := orientation(a, b, c)
	o2 := orientation(a, b, d)
	o3 := orientation(c, d, a)
	o4 := orientation(c, d, b)
	return o1*o2 < -epsilon && o3*o4 < -epsilon
}

func segmentsIntersect(a, b, c, d Point) bool {
	if segmentsProperlyIntersect(a, b, c, d) {
		return true
	}
	return pointOnSegment(c, a, b) || pointOnSegment(d, a, b) ||
		pointOnSegment(a, c, d) || pointOnSegment(b, c, d)
}
