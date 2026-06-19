package engine

import (
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
)

type Biome struct {
	ID          BiomeID
	Name        string
	Temperature float64
	Humidity    float64
	Terrain     TerrainType

	ResourceProfile ResourceProfile
}

func NewBiome(
	name string,
	temperature, humidity float64,
	terrain TerrainType,
	profile ResourceProfile,
) (*Biome, error) {
	if profile == nil {
		profile = DefaultResourceProfile(terrain)
	}
	biome := &Biome{
		ID:              BiomeID(uuid.NewString()),
		Name:            name,
		Temperature:     temperature,
		Humidity:        humidity,
		Terrain:         terrain,
		ResourceProfile: profile.Clone(),
	}
	if err := biome.Validate(); err != nil {
		return nil, err
	}
	return biome, nil
}

func (b *Biome) Validate() error {
	if b == nil {
		return errors.New("biome cannot be nil")
	}
	if b.ID == "" {
		return errors.New("biome ID cannot be empty")
	}
	if b.Name == "" {
		return errors.New("biome name cannot be empty")
	}
	if b.Temperature < -100 || b.Temperature > 100 {
		return fmt.Errorf("temperature must be between -100 and 100: %v", b.Temperature)
	}
	if b.Humidity < 0 || b.Humidity > 1 {
		return fmt.Errorf("humidity must be between 0 and 1: %v", b.Humidity)
	}
	if !b.Terrain.Valid() {
		return fmt.Errorf("invalid terrain: %d", b.Terrain)
	}
	if err := b.ResourceProfile.Validate(); err != nil {
		return fmt.Errorf("invalid resource profile: %w", err)
	}
	return nil
}

func (b *Biome) ResourcesForArea(area float64) (Resources, error) {
	if area <= 0 {
		return nil, fmt.Errorf("area must be positive: %v", area)
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}

	resources := make(Resources, len(b.ResourceProfile))
	for kind, propensity := range b.ResourceProfile {
		capacity := area * propensity.CapacityPerArea * b.resourceMultiplier(kind)
		resources[kind] = &ResourceStock{
			Amount:       capacity * propensity.InitialDensity,
			Capacity:     capacity,
			RegenPerTick: capacity * propensity.RegenerationRate,
		}
	}
	return resources, nil
}

func (b *Biome) resourceMultiplier(kind ResourceType) float64 {
	traits := b.Terrain.Traits()
	temperatureSuitability := clamp(1-math.Abs(b.Temperature-20)/50, 0.1, 1)

	switch kind {
	case ResourceWater:
		return clamp((0.5+b.Humidity)*traits.WaterRetention, 0.1, 2)
	case ResourceFood:
		return clamp(traits.Fertility*temperatureSuitability*(0.5+b.Humidity), 0.05, 2)
	case ResourceWood:
		return clamp(traits.WoodGrowth*(0.5+b.Humidity), 0.01, 2)
	case ResourceStone:
		return clamp(traits.StoneRichness, 0.1, 2)
	default:
		return 1
	}
}

type TerrainType int

const (
	TerrainTypePlains TerrainType = iota
	TerrainTypeForest
	TerrainTypeMountain
	TerrainTypeDesert
	TerrainTypeWetland
)

func (t TerrainType) Valid() bool {
	return t >= TerrainTypePlains && t <= TerrainTypeWetland
}

type TerrainTraits struct {
	MovementCost   float64
	Fertility      float64
	WaterRetention float64
	WoodGrowth     float64
	StoneRichness  float64
}

func (t TerrainType) Traits() TerrainTraits {
	switch t {
	case TerrainTypeForest:
		return TerrainTraits{MovementCost: 1.4, Fertility: 1.3, WaterRetention: 1.2, WoodGrowth: 1.6, StoneRichness: 0.6}
	case TerrainTypeMountain:
		return TerrainTraits{MovementCost: 2.2, Fertility: 0.4, WaterRetention: 0.7, WoodGrowth: 0.4, StoneRichness: 1.8}
	case TerrainTypeDesert:
		return TerrainTraits{MovementCost: 1.6, Fertility: 0.2, WaterRetention: 0.2, WoodGrowth: 0.1, StoneRichness: 1.1}
	case TerrainTypeWetland:
		return TerrainTraits{MovementCost: 1.8, Fertility: 1.5, WaterRetention: 1.8, WoodGrowth: 1.0, StoneRichness: 0.3}
	default:
		return TerrainTraits{MovementCost: 1, Fertility: 1, WaterRetention: 1, WoodGrowth: 0.7, StoneRichness: 0.8}
	}
}
