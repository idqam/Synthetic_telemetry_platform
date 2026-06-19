package engine

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/google/uuid"
)

const epsilon = 1e-9

var (
	ErrInsufficientResource = errors.New("insufficient resource")
	ErrRegionCapacity       = errors.New("region capacity reached")
)

type WorldMapID string
type RegionID string
type BiomeID string
type EntityID string

type TimeMode int

const (
	TimeModeDiscrete TimeMode = iota
	TimeModeContinuous
)

func (m TimeMode) Valid() bool {
	return m == TimeModeDiscrete || m == TimeModeContinuous
}

type WorldMap struct {
	ID     WorldMapID
	Height int
	Width  int

	Tick    uint64
	SimTime float64

	TimeMode TimeMode

	Regions []*Region
	Biomes  map[BiomeID]*Biome

	InitialEntityCount int
	CurrentEntityCount int
}

func NewWorldMap(height, width int, timeMode TimeMode, initialEntityCount int) (*WorldMap, error) {
	if height <= 0 || width <= 0 {
		return nil, fmt.Errorf("map dimensions must be positive: width=%d height=%d", width, height)
	}
	if !timeMode.Valid() {
		return nil, fmt.Errorf("invalid time mode: %d", timeMode)
	}
	if initialEntityCount < 0 {
		return nil, fmt.Errorf("initial entity count cannot be negative: %d", initialEntityCount)
	}

	return &WorldMap{
		ID:                 WorldMapID(uuid.NewString()),
		Height:             height,
		Width:              width,
		TimeMode:           timeMode,
		Regions:            make([]*Region, 0),
		Biomes:             make(map[BiomeID]*Biome),
		InitialEntityCount: initialEntityCount,
		CurrentEntityCount: initialEntityCount,
	}, nil
}

func NewRandomWorldMap() (*WorldMap, error) {
	height := 64 + rand.IntN(193)
	width := 64 + rand.IntN(193)
	rows := 2 + rand.IntN(4)
	columns := 2 + rand.IntN(4)
	capacityPerRegion := 50 + rand.IntN(201)
	regionCount := rows * columns
	initialEntityCount := rand.IntN(regionCount*capacityPerRegion + 1)
	timeMode := TimeMode(rand.IntN(2))

	world, err := NewWorldMap(height, width, timeMode, initialEntityCount)
	if err != nil {
		return nil, err
	}

	biomeCount := 2 + rand.IntN(4)
	biomeIDs := make([]BiomeID, 0, biomeCount)
	terrainNames := [...]string{"plains", "forest", "mountain", "desert", "wetland"}
	temperatures := [...]float64{20, 16, 5, 34, 22}
	humidities := [...]float64{0.5, 0.75, 0.35, 0.12, 0.85}

	for i := 0; i < biomeCount; i++ {
		terrain := TerrainType(rand.IntN(len(terrainNames)))
		temperature := temperatures[terrain] + rand.Float64()*10 - 5
		humidity := humidities[terrain] + rand.Float64()*0.1 - 0.05
		name := fmt.Sprintf("%s-%d", terrainNames[terrain], i+1)
		biome, err := NewBiome(name, temperature, humidity, terrain, nil)
		if err != nil {
			return nil, fmt.Errorf("generate biome: %w", err)
		}
		if err := world.AddBiome(biome); err != nil {
			return nil, fmt.Errorf("add generated biome: %w", err)
		}
		biomeIDs = append(biomeIDs, biome.ID)
	}

	regions, err := NewGridRegions(width, height, rows, columns, biomeIDs, capacityPerRegion)
	if err != nil {
		return nil, fmt.Errorf("generate regions: %w", err)
	}
	for _, region := range regions {
		if err := world.AddRegion(region); err != nil {
			return nil, fmt.Errorf("add generated region: %w", err)
		}
	}
	if err := world.ValidatePartition(); err != nil {
		return nil, fmt.Errorf("validate generated map: %w", err)
	}
	return world, nil
}

func (m *WorldMap) Bounds() Bounds {
	return Bounds{MinX: 0, MinY: 0, MaxX: float64(m.Width), MaxY: float64(m.Height)}
}

func (m *WorldMap) AddBiome(biome *Biome) error {
	if m == nil {
		return errors.New("map cannot be nil")
	}
	if biome == nil {
		return errors.New("biome cannot be nil")
	}
	if err := biome.Validate(); err != nil {
		return fmt.Errorf("invalid biome: %w", err)
	}
	if m.Biomes == nil {
		m.Biomes = make(map[BiomeID]*Biome)
	}
	if _, exists := m.Biomes[biome.ID]; exists {
		return fmt.Errorf("biome %q already exists", biome.ID)
	}
	m.Biomes[biome.ID] = biome
	return nil
}

func (m *WorldMap) AddRegion(region *Region) error {
	if m == nil {
		return errors.New("map cannot be nil")
	}
	if region == nil {
		return errors.New("region cannot be nil")
	}
	if err := region.Validate(); err != nil {
		return fmt.Errorf("invalid region: %w", err)
	}
	if m.regionByID(region.ID) != nil {
		return fmt.Errorf("region %q already exists", region.ID)
	}
	if !m.Bounds().ContainsBounds(region.Shape.Bounds()) {
		return fmt.Errorf("region %q is outside map bounds", region.ID)
	}

	for _, share := range region.Biomes {
		if _, exists := m.Biomes[share.BiomeID]; !exists {
			return fmt.Errorf("region %q references unknown biome %q", region.ID, share.BiomeID)
		}
	}

	for _, existing := range m.Regions {
		if region.Shape.OverlapsArea(existing.Shape) {
			return fmt.Errorf("region %q overlaps region %q", region.ID, existing.ID)
		}
	}

	if m.CoveredArea()+region.Shape.Area() > float64(m.Width*m.Height)+epsilon {
		return fmt.Errorf("adding region %q would exceed map area", region.ID)
	}

	if len(region.Resources) == 0 {
		resources, err := resourcesForBiomeMix(region.Shape.Area(), region.Biomes, m.Biomes)
		if err != nil {
			return fmt.Errorf("initialize region resources: %w", err)
		}
		region.Resources = resources
	}

	m.Regions = append(m.Regions, region)
	return nil
}

func (m *WorldMap) RegionAt(point Point) (*Region, bool) {
	for _, region := range m.Regions {
		if region.Shape.Contains(point) {
			return region, true
		}
	}
	return nil, false
}

func (m *WorldMap) CoveredArea() float64 {
	var total float64
	for _, region := range m.Regions {
		total += region.Shape.Area()
	}
	return total
}

func (m *WorldMap) CoverageRatio() float64 {
	return m.CoveredArea() / float64(m.Width*m.Height)
}

func (m *WorldMap) Advance(delta float64) error {
	if delta <= 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return fmt.Errorf("delta must be finite and positive: %v", delta)
	}

	switch m.TimeMode {
	case TimeModeDiscrete:
		if math.Abs(delta-math.Round(delta)) > epsilon {
			return fmt.Errorf("discrete time requires a whole-number delta: %v", delta)
		}
		m.Tick += uint64(math.Round(delta))
	case TimeModeContinuous:
		m.Tick++
	default:
		return fmt.Errorf("invalid time mode: %d", m.TimeMode)
	}

	m.SimTime += delta
	for _, region := range m.Regions {
		region.Resources.Regenerate(delta)
	}
	return nil
}

func (m *WorldMap) Validate() error {
	if m == nil {
		return errors.New("map cannot be nil")
	}
	if m.ID == "" {
		return errors.New("map ID cannot be empty")
	}
	if m.Width <= 0 || m.Height <= 0 {
		return errors.New("map dimensions must be positive")
	}
	if !m.TimeMode.Valid() {
		return fmt.Errorf("invalid time mode: %d", m.TimeMode)
	}
	if m.InitialEntityCount < 0 || m.CurrentEntityCount < 0 {
		return errors.New("entity counts cannot be negative")
	}

	seen := make(map[RegionID]struct{}, len(m.Regions))
	for i, region := range m.Regions {
		if region == nil {
			return fmt.Errorf("region at index %d is nil", i)
		}
		if _, exists := seen[region.ID]; exists {
			return fmt.Errorf("duplicate region ID %q", region.ID)
		}
		seen[region.ID] = struct{}{}
		if err := region.Validate(); err != nil {
			return fmt.Errorf("region %q: %w", region.ID, err)
		}
		if !m.Bounds().ContainsBounds(region.Shape.Bounds()) {
			return fmt.Errorf("region %q is outside map bounds", region.ID)
		}
		for _, share := range region.Biomes {
			if _, exists := m.Biomes[share.BiomeID]; !exists {
				return fmt.Errorf("region %q references unknown biome %q", region.ID, share.BiomeID)
			}
		}
		for j := i + 1; j < len(m.Regions); j++ {
			if m.Regions[j] == nil {
				return fmt.Errorf("region at index %d is nil", j)
			}
			if region.Shape.OverlapsArea(m.Regions[j].Shape) {
				return fmt.Errorf("regions %q and %q overlap", region.ID, m.Regions[j].ID)
			}
		}
	}

	if m.CoveredArea() > float64(m.Width*m.Height)+epsilon {
		return errors.New("region area exceeds map area")
	}
	return nil
}

func (m *WorldMap) ValidatePartition() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if math.Abs(m.CoverageRatio()-1) > epsilon {
		return fmt.Errorf("regions cover %.4f of the map; expected 1.0", m.CoverageRatio())
	}
	return nil
}

func (m *WorldMap) regionByID(id RegionID) *Region {
	for _, region := range m.Regions {
		if region.ID == id {
			return region
		}
	}
	return nil
}

func NewGridRegions(
	width, height, rows, columns int,
	biomeIDs []BiomeID,
	capacityPerRegion int,
) ([]*Region, error) {
	if width <= 0 || height <= 0 || rows <= 0 || columns <= 0 {
		return nil, errors.New("width, height, rows, and columns must be positive")
	}
	if len(biomeIDs) == 0 {
		return nil, errors.New("at least one biome ID is required")
	}
	if capacityPerRegion <= 0 {
		return nil, errors.New("capacity must be positive")
	}

	cellWidth := float64(width) / float64(columns)
	cellHeight := float64(height) / float64(rows)
	regions := make([]*Region, 0, rows*columns)

	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			minX := float64(column) * cellWidth
			minY := float64(row) * cellHeight
			maxX := float64(column+1) * cellWidth
			maxY := float64(row+1) * cellHeight
			shape, err := NewRectangle(minX, minY, maxX, maxY)
			if err != nil {
				return nil, err
			}
			biomeID := biomeIDs[(row*columns+column)%len(biomeIDs)]
			region, err := NewRegion(shape, capacityPerRegion, []BiomeShare{{
				BiomeID:  biomeID,
				Fraction: 1,
			}})
			if err != nil {
				return nil, err
			}
			regions = append(regions, region)
		}
	}
	return regions, nil
}
