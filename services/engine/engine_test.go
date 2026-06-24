package engine

import (
	"math"
	"testing"
)

func newTestEngine(t *testing.T, capacity int) (*Engine, *Region, *Species) {
	t.Helper()

	world, err := NewWorldMap(10, 10, TimeModeContinuous, 0)
	if err != nil {
		t.Fatal(err)
	}
	biome, err := NewBiome("plains", 20, 0.5, TerrainTypePlains, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(biome); err != nil {
		t.Fatal(err)
	}
	shape, err := NewRectangle(0, 0, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	region, err := NewRegion(shape, capacity, []BiomeShare{{BiomeID: biome.ID, Fraction: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(region); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(world, 1)
	species := NewSpecies("rabbit")
	if err := engine.RegisterSpecies(species); err != nil {
		t.Fatal(err)
	}
	return engine, region, species
}

func TestSpawnOrganismIndexesByRegion(t *testing.T) {
	engine, region, species := newTestEngine(t, 100)

	organism, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: 1, Y: 1})
	if err != nil {
		t.Fatal(err)
	}

	members := engine.OrganismsByRegion[region.ID]
	if _, ok := members[organism.ID]; !ok {
		t.Fatalf("expected organism %q to be indexed under region %q", organism.ID, region.ID)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 indexed organism, got %d", len(members))
	}
}

func TestAdvanceRegionRegeneratesResources(t *testing.T) {
	engine, region, _ := newTestEngine(t, 100)

	region.Resources.RemoveUpTo(ResourceFood, math.MaxFloat64)
	if before := region.Resources.Amount(ResourceFood); before != 0 {
		t.Fatalf("expected food fully depleted, got %v", before)
	}

	if err := engine.AdvanceRegion(region.ID, 1); err != nil {
		t.Fatal(err)
	}

	if after := region.Resources.Amount(ResourceFood); after <= 0 {
		t.Fatalf("expected food to regenerate, got %v", after)
	}
}

func TestAdvanceRegionAppliesMetabolism(t *testing.T) {
	engine, region, species := newTestEngine(t, 100)

	organism, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: 1, Y: 1})
	if err != nil {
		t.Fatal(err)
	}
	before := organism.Energy.Current

	if err := engine.AdvanceRegion(region.ID, 2); err != nil {
		t.Fatal(err)
	}

	want := before - species.MetabolismRate*2
	if math.Abs(organism.Energy.Current-want) > epsilon {
		t.Fatalf("expected energy %v after metabolism, got %v", want, organism.Energy.Current)
	}
}

func TestAdvanceRegionStressMatchesOccupancy(t *testing.T) {
	engine, region, species := newTestEngine(t, 4)

	a, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: 1, Y: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: 2, Y: 2})
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.AdvanceRegion(region.ID, 1); err != nil {
		t.Fatal(err)
	}

	const wantStress = 0.5
	for _, o := range []*Organism{a, b} {
		if math.Abs(o.Vitals.Stress-wantStress) > epsilon {
			t.Fatalf("expected stress %v from occupancy, got %v", wantStress, o.Vitals.Stress)
		}
	}
}

func TestAdvanceRegionRemovesDeadFromIndex(t *testing.T) {
	engine, region, species := newTestEngine(t, 100)

	organism, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: 1, Y: 1})
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.AdvanceRegion(region.ID, 100); err != nil {
		t.Fatal(err)
	}

	if organism.Alive() {
		t.Fatal("expected organism to starve")
	}
	if organism.DeathCause != DeathCauseStarvation {
		t.Fatalf("expected starvation death, got %q", organism.DeathCause)
	}
	if _, ok := engine.OrganismsByRegion[region.ID][organism.ID]; ok {
		t.Fatal("expected dead organism to be removed from region index")
	}
	if _, ok := engine.Organisms[organism.ID]; !ok {
		t.Fatal("dead organism should remain in the organism store")
	}
}

func TestAdvanceRegionConcurrentIndexConsistency(t *testing.T) {
	engine, region, species := newTestEngine(t, 1000)

	const population = 500
	for i := 0; i < population; i++ {
		x := float64(i%9) + 0.5
		y := float64(i%9) + 0.5
		if _, err := engine.SpawnOrganism(species.ID, region.ID, Point{X: x, Y: y}); err != nil {
			t.Fatal(err)
		}
	}

	if err := engine.AdvanceRegion(region.ID, species.MaxEnergy+1); err != nil {
		t.Fatal(err)
	}

	alive := 0
	for _, o := range engine.Organisms {
		if o.Alive() {
			alive++
		}
	}
	if alive != 0 {
		t.Fatalf("expected all organisms to starve, %d still alive", alive)
	}
	if got := len(engine.OrganismsByRegion[region.ID]); got != 0 {
		t.Fatalf("expected region index emptied after mass death, got %d", got)
	}
}

func TestMoveOrganismRekeysIndex(t *testing.T) {
	world, err := NewWorldMap(10, 10, TimeModeContinuous, 0)
	if err != nil {
		t.Fatal(err)
	}
	biome, err := NewBiome("plains", 20, 0.5, TerrainTypePlains, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(biome); err != nil {
		t.Fatal(err)
	}
	left, err := NewRectangle(0, 0, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewRectangle(5, 0, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	regionA, err := NewRegion(left, 100, []BiomeShare{{BiomeID: biome.ID, Fraction: 1}})
	if err != nil {
		t.Fatal(err)
	}
	regionB, err := NewRegion(right, 100, []BiomeShare{{BiomeID: biome.ID, Fraction: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(regionA); err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(regionB); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(world, 1)
	species := NewSpecies("rabbit")
	if err := engine.RegisterSpecies(species); err != nil {
		t.Fatal(err)
	}

	organism, err := engine.SpawnOrganism(species.ID, regionA.ID, Point{X: 4.5, Y: 5})
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.MoveOrganism(organism, Point{X: 5.5, Y: 5}, regionB.ID); err != nil {
		t.Fatal(err)
	}

	if organism.RegionID != regionB.ID {
		t.Fatalf("expected organism in region B, got %q", organism.RegionID)
	}
	if _, ok := engine.OrganismsByRegion[regionA.ID][organism.ID]; ok {
		t.Fatal("expected organism removed from source region index")
	}
	if _, ok := engine.OrganismsByRegion[regionB.ID][organism.ID]; !ok {
		t.Fatal("expected organism added to destination region index")
	}
}
