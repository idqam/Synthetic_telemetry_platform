package engine

import "testing"

func TestNewRandomWorldMap(t *testing.T) {
	for range 10 {
		world, err := NewRandomWorldMap()
		if err != nil {
			t.Fatal(err)
		}
		if err := world.ValidatePartition(); err != nil {
			t.Fatal(err)
		}
		if world.Width < 64 || world.Width > 256 || world.Height < 64 || world.Height > 256 {
			t.Fatalf("unexpected dimensions: width=%d height=%d", world.Width, world.Height)
		}
		if len(world.Biomes) < 2 || len(world.Biomes) > 5 {
			t.Fatalf("unexpected biome count: %d", len(world.Biomes))
		}
		if len(world.Regions) < 4 || len(world.Regions) > 25 {
			t.Fatalf("unexpected region count: %d", len(world.Regions))
		}
		if world.InitialEntityCount != world.CurrentEntityCount {
			t.Fatalf(
				"entity counts do not match: initial=%d current=%d",
				world.InitialEntityCount,
				world.CurrentEntityCount,
			)
		}
		for _, region := range world.Regions {
			if len(region.Resources) == 0 {
				t.Fatalf("region %q has no resources", region.ID)
			}
		}
	}
}

func TestGridRegionsFormPartitionAndRegenerate(t *testing.T) {
	world, err := NewWorldMap(100, 100, TimeModeDiscrete, 0)
	if err != nil {
		t.Fatal(err)
	}

	forest, err := NewBiome("forest", 18, 0.8, TerrainTypeForest, nil)
	if err != nil {
		t.Fatal(err)
	}
	desert, err := NewBiome("desert", 33, 0.1, TerrainTypeDesert, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(forest); err != nil {
		t.Fatal(err)
	}
	if err := world.AddBiome(desert); err != nil {
		t.Fatal(err)
	}

	regions, err := NewGridRegions(100, 100, 2, 2, []BiomeID{forest.ID, desert.ID}, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range regions {
		if err := world.AddRegion(region); err != nil {
			t.Fatal(err)
		}
	}
	if err := world.ValidatePartition(); err != nil {
		t.Fatal(err)
	}

	before := regions[0].Resources.Amount(ResourceFood)
	if err := regions[0].Resources.Remove(ResourceFood, 1); err != nil {
		t.Fatal(err)
	}
	if err := world.Advance(1); err != nil {
		t.Fatal(err)
	}
	if got := regions[0].Resources.Amount(ResourceFood); got <= before-1 {
		t.Fatalf("expected regeneration, before=%v after=%v", before, got)
	}
}
