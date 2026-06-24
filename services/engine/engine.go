package engine

type Engine struct {
	World *WorldMap

	Species   map[SpeciesID]*Species
	Organisms map[EntityID]*Organism
}
