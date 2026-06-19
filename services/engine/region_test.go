package engine

import (
	"math"
	"testing"
)

func TestIrregularRegionAndOverlap(t *testing.T) {
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

	triangle, err := NewPolygon([]Point{{0, 0}, {5, 0}, {0, 5}})
	if err != nil {
		t.Fatal(err)
	}
	region, err := NewRegion(triangle, 10, []BiomeShare{{BiomeID: biome.ID, Fraction: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(region); err != nil {
		t.Fatal(err)
	}

	overlap, err := NewRectangle(1, 1, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	overlappingRegion, err := NewRegion(overlap, 10, []BiomeShare{{BiomeID: biome.ID, Fraction: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.AddRegion(overlappingRegion); err == nil {
		t.Fatal("expected overlapping region to be rejected")
	}

	if _, ok := world.RegionAt(Point{X: 1, Y: 1}); !ok {
		t.Fatal("expected point to be in triangle")
	}
	if err := world.Advance(0.25); err != nil {
		t.Fatal(err)
	}
	if math.Abs(world.SimTime-0.25) > epsilon || world.Tick != 1 {
		t.Fatalf("unexpected time state: simTime=%v tick=%d", world.SimTime, world.Tick)
	}
}
