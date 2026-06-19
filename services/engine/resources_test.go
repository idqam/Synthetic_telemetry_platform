package engine

import "testing"

func TestMixedBiomeResources(t *testing.T) {
	world, err := NewWorldMap(10, 10, TimeModeDiscrete, 0)
	if err != nil {
		t.Fatal(err)
	}
	forest, err := NewBiome("forest", 18, 0.8, TerrainTypeForest, nil)
	if err != nil {
		t.Fatal(err)
	}
	desert, err := NewBiome("desert", 35, 0.1, TerrainTypeDesert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(forest); err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(desert); err != nil {
		t.Fatal(err)
	}

	shape, err := NewRectangle(0, 0, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	region, err := NewRegion(shape, 100, []BiomeShare{
		{BiomeID: forest.ID, Fraction: 0.7},
		{BiomeID: desert.ID, Fraction: 0.3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(region); err != nil {
		t.Fatal(err)
	}
	if region.DominantBiome() != forest.ID {
		t.Fatal("forest should be dominant")
	}
	if region.Resources.Amount(ResourceWater) <= 0 || region.Resources.Amount(ResourceWood) <= 0 {
		t.Fatal("expected initialized resources")
	}
}

func TestRemoveUpToAvailableResourceAmount(t *testing.T) {
	resources := Resources{
		ResourceWater: {
			Amount:   3,
			Capacity: 10,
		},
	}

	if removed := resources.RemoveUpTo(ResourceWater, 5); removed != 3 {
		t.Fatalf("expected to remove 3, removed %v", removed)
	}
	if remaining := resources.Amount(ResourceWater); remaining != 0 {
		t.Fatalf("expected no water remaining, got %v", remaining)
	}
}
