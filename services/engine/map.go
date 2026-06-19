package engine

// ALL IDS SHOULD BE UUID STRINGS TO ENSURE UNIQUENESS ACROSS THE SYSTEM
type Map struct {
	ID     MapID
	Height int
	Width  int
	Tick   uint64

	TimeMode TimeMode

	Regions []Region
	Biomes  map[BiomeID]*Biome

	InitialEntityCount int
	CurrentEntityCount int
}

type MapID string

type TimeMode int

const (
	TimeModeDiscrete TimeMode = iota
	TimeModeContinous
)

type Region struct {
	ID      RegionID
	X       int
	Y       int
	Width   int
	Height  int
	BiomeID BiomeID

	Capacity  int
	Resources Resources
}
type RegionID string

type Biome struct {
	ID          BiomeID
	Name        string
	Temperature float64
	Humidity    float64
	Terrain     TerrainType

	ResourceProfile ResourceProfile
}

type TerrainType int

const (
	TerrainTypePlains TerrainType = iota
	TerrainTypeForest
	TerrainTypeMountain
	TerrainTypeDesert
)

type ResourceProfile struct {
	WaterDensity float64 //0 to 1, representing the abundance of water in the biome
	FoodDensity  float64 //0 to 1, representing the abundance of food in the biome
	WoodDensity  float64 //0 to 1, representing the abundance of wood in the biome
	StoneDensity float64 //0 to 1, representing the abundance of stone in the biome
}

type Resources struct {
	Water string
	Food  string
	Wood  string
	Stone string
}
type BiomeID string

type EntityID string
