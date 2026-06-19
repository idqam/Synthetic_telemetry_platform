package engine

import (
	"errors"
	"fmt"
	"math"
)

type ResourceType string

const (
	ResourceWater ResourceType = "water"
	ResourceFood  ResourceType = "food"
	ResourceWood  ResourceType = "wood"
	ResourceStone ResourceType = "stone"
)

type ResourcePropensity struct {
	InitialDensity   float64
	CapacityPerArea  float64
	RegenerationRate float64
}

type ResourceProfile map[ResourceType]ResourcePropensity

func DefaultResourceProfile(terrain TerrainType) ResourceProfile {
	profile := ResourceProfile{
		ResourceWater: {InitialDensity: 0.65, CapacityPerArea: 8, RegenerationRate: 0.010},
		ResourceFood:  {InitialDensity: 0.55, CapacityPerArea: 6, RegenerationRate: 0.025},
		ResourceWood:  {InitialDensity: 0.60, CapacityPerArea: 5, RegenerationRate: 0.006},
		ResourceStone: {InitialDensity: 0.85, CapacityPerArea: 7, RegenerationRate: 0},
	}

	if terrain == TerrainTypeDesert {
		profile[ResourceWater] = ResourcePropensity{InitialDensity: 0.2, CapacityPerArea: 3, RegenerationRate: 0.002}
		profile[ResourceFood] = ResourcePropensity{InitialDensity: 0.2, CapacityPerArea: 2, RegenerationRate: 0.008}
	}
	if terrain == TerrainTypeForest {
		profile[ResourceWood] = ResourcePropensity{InitialDensity: 0.8, CapacityPerArea: 10, RegenerationRate: 0.012}
	}
	if terrain == TerrainTypeMountain {
		profile[ResourceStone] = ResourcePropensity{InitialDensity: 0.95, CapacityPerArea: 12, RegenerationRate: 0}
	}
	return profile
}

func (p ResourceProfile) Validate() error {
	if len(p) == 0 {
		return errors.New("resource profile cannot be empty")
	}
	for kind, propensity := range p {
		if kind == "" {
			return errors.New("resource type cannot be empty")
		}
		if propensity.InitialDensity < 0 || propensity.InitialDensity > 1 {
			return fmt.Errorf("%s initial density must be in [0, 1]", kind)
		}
		if propensity.CapacityPerArea < 0 {
			return fmt.Errorf("%s capacity per area cannot be negative", kind)
		}
		if propensity.RegenerationRate < 0 || propensity.RegenerationRate > 1 {
			return fmt.Errorf("%s regeneration rate must be in [0, 1]", kind)
		}
	}
	return nil
}

func (p ResourceProfile) Clone() ResourceProfile {
	clone := make(ResourceProfile, len(p))
	for kind, propensity := range p {
		clone[kind] = propensity
	}
	return clone
}

type ResourceStock struct {
	Amount       float64
	Capacity     float64
	RegenPerTick float64
}

type Resources map[ResourceType]*ResourceStock

func (r Resources) Validate() error {
	for kind, stock := range r {
		if kind == "" {
			return errors.New("resource type cannot be empty")
		}
		if stock == nil {
			return fmt.Errorf("resource %q has nil stock", kind)
		}
		if stock.Amount < -epsilon || stock.Capacity < 0 || stock.RegenPerTick < 0 {
			return fmt.Errorf("resource %q has negative values", kind)
		}
		if stock.Amount > stock.Capacity+epsilon {
			return fmt.Errorf("resource %q amount exceeds capacity", kind)
		}
	}
	return nil
}

func (r Resources) Amount(kind ResourceType) float64 {
	if stock := r[kind]; stock != nil {
		return stock.Amount
	}
	return 0
}

func (r Resources) Remove(kind ResourceType, amount float64) error {
	if amount < 0 {
		return errors.New("remove amount cannot be negative")
	}
	stock := r[kind]
	if stock == nil || stock.Amount+epsilon < amount {
		return fmt.Errorf("%w: need %.3f %s, have %.3f", ErrInsufficientResource, amount, kind, r.Amount(kind))
	}
	stock.Amount = math.Max(0, stock.Amount-amount)
	return nil
}

func (r Resources) RemoveUpTo(kind ResourceType, amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	stock := r[kind]
	if stock == nil {
		return 0
	}
	removed := math.Min(stock.Amount, amount)
	stock.Amount -= removed
	return removed
}

func (r Resources) Add(kind ResourceType, amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	stock := r[kind]
	if stock == nil {
		stock = &ResourceStock{Capacity: amount}
		r[kind] = stock
	}
	added := math.Min(amount, stock.Capacity-stock.Amount)
	stock.Amount += added
	return added
}

func (r Resources) Regenerate(delta float64) {
	if delta <= 0 {
		return
	}
	for _, stock := range r {
		if stock == nil || stock.RegenPerTick <= 0 {
			continue
		}
		stock.Amount = math.Min(stock.Capacity, stock.Amount+stock.RegenPerTick*delta)
	}
}

func (r Resources) Scarcity(kind ResourceType) float64 {
	stock := r[kind]
	if stock == nil || stock.Capacity <= 0 {
		return 1
	}
	return clamp(1-stock.Amount/stock.Capacity, 0, 1)
}

func (r Resources) Depleted(kind ResourceType, threshold float64) bool {
	return r.Scarcity(kind) >= clamp(threshold, 0, 1)
}

func (r Resources) Merge(other Resources) {
	for kind, incoming := range other {
		if incoming == nil {
			continue
		}
		stock := r[kind]
		if stock == nil {
			r[kind] = &ResourceStock{
				Amount:       incoming.Amount,
				Capacity:     incoming.Capacity,
				RegenPerTick: incoming.RegenPerTick,
			}
			continue
		}
		stock.Amount += incoming.Amount
		stock.Capacity += incoming.Capacity
		stock.RegenPerTick += incoming.RegenPerTick
	}
}

func (r Resources) Clone() Resources {
	clone := make(Resources, len(r))
	for kind, stock := range r {
		if stock != nil {
			copy := *stock
			clone[kind] = &copy
		}
	}
	return clone
}

func resourcesForBiomeMix(area float64, shares []BiomeShare, biomes map[BiomeID]*Biome) (Resources, error) {
	resources := make(Resources)
	for _, share := range shares {
		biome := biomes[share.BiomeID]
		if biome == nil {
			return nil, fmt.Errorf("unknown biome %q", share.BiomeID)
		}
		biomeResources, err := biome.ResourcesForArea(area * share.Fraction)
		if err != nil {
			return nil, err
		}
		resources.Merge(biomeResources)
	}
	return resources, nil
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
