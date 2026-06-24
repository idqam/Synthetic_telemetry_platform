package engine

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
)

type Engine struct {
	World *WorldMap

	Species           map[SpeciesID]*Species
	Organisms         map[EntityID]*Organism
	OrganismsByRegion map[RegionID]map[EntityID]struct{}

	Rng *rand.Rand
}

func NewEngine(world *WorldMap, seed uint64) *Engine {
	return &Engine{
		World:             world,
		Species:           make(map[SpeciesID]*Species),
		Organisms:         make(map[EntityID]*Organism),
		OrganismsByRegion: make(map[RegionID]map[EntityID]struct{}),

		Rng: rand.New(rand.NewPCG(seed, seed)),
	}
}

func (e *Engine) RegisterSpecies(species *Species) error {
	if species == nil {
		return errors.New("species cannot be nil")
	}
	if err := species.Validate(); err != nil {
		return fmt.Errorf("invalid species: %w", err)
	}
	if _, exists := e.Species[species.ID]; exists {
		return fmt.Errorf("species %q already registered", species.ID)
	}
	e.Species[species.ID] = species
	return nil
}

func (e *Engine) SpawnOrganism(speciesID SpeciesID, regionID RegionID, position Point) (*Organism, error) {
	species, ok := e.Species[speciesID]
	if !ok {
		return nil, fmt.Errorf("unknown species %q", speciesID)
	}

	region := e.World.regionByID(regionID)
	if region == nil {
		return nil, fmt.Errorf("unknown region %q", regionID)
	}
	if !region.Shape.Contains(position) {
		return nil, fmt.Errorf("position is outside region %q", regionID)
	}

	organism, err := NewOrganism(
		species,
		regionID,
		position,
		e.World.Tick,
		e.World.SimTime,
		nil,
		0,
	)
	if err != nil {
		return nil, err
	}

	e.Organisms[organism.ID] = organism
	e.indexOrganism(organism)
	return organism, nil
}

func (e *Engine) MoveOrganism(organism *Organism, destination Point, destRegionID RegionID) error {
	if organism == nil {
		return errors.New("organism cannot be nil")
	}
	species, ok := e.Species[organism.SpeciesID]
	if !ok {
		return fmt.Errorf("unknown species %q", organism.SpeciesID)
	}
	destRegion := e.World.regionByID(destRegionID)
	if destRegion == nil {
		return fmt.Errorf("unknown region %q", destRegionID)
	}
	if !destRegion.Shape.Contains(destination) {
		return fmt.Errorf("destination is outside region %q", destRegionID)
	}

	from := organism.RegionID
	if err := organism.ApplyMovement(destination, destRegionID, species); err != nil {
		return err
	}
	if from != organism.RegionID {
		e.removeFromRegion(from, organism.ID)
		e.indexOrganism(organism)
	}
	return nil
}

func (e *Engine) KillOrganism(organism *Organism, cause DeathCause) error {
	if organism == nil {
		return errors.New("organism cannot be nil")
	}
	if err := organism.Die(cause, e.World.Tick); err != nil {
		return err
	}
	e.deindexOrganism(organism)
	return nil
}

func (e *Engine) AdvanceRegion(regionID RegionID, delta float64) error {
	if !finite(delta) || delta <= 0 {
		return errors.New("delta must be finite and positive")
	}
	region := e.World.regionByID(regionID)
	if region == nil {
		return fmt.Errorf("unknown region %q", regionID)
	}

	region.Resources.Regenerate(delta)

	members := e.OrganismsByRegion[regionID]
	ids := make([]EntityID, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	stress := region.OccupancyRatio(len(ids))

	workers := runtime.NumCPU()
	if workers > len(ids) {
		workers = len(ids)
	}

	jobs := make(chan EntityID)
	dead := make(chan EntityID, len(ids))
	errs := make(chan error, len(ids))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				organism := e.Organisms[id]
				if organism == nil || !organism.Alive() {
					continue
				}
				if err := e.advanceOrganism(organism, stress, delta); err != nil {
					errs <- err
					continue
				}
				if !organism.Alive() {
					dead <- id
				}
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	close(dead)
	close(errs)

	for id := range dead {
		if organism := e.Organisms[id]; organism != nil {
			e.deindexOrganism(organism)
		}
	}
	if err := <-errs; err != nil {
		return err
	}
	return nil
}

func (e *Engine) advanceOrganism(organism *Organism, stress, delta float64) error {
	species, ok := e.Species[organism.SpeciesID]
	if !ok {
		return fmt.Errorf("unknown species %q", organism.SpeciesID)
	}
	organism.ApplyCrowdingStress(stress)
	return organism.Advance(e.World.Tick, e.World.SimTime, delta, species)
}

func (e *Engine) indexOrganism(organism *Organism) {
	set := e.OrganismsByRegion[organism.RegionID]
	if set == nil {
		set = make(map[EntityID]struct{})
		e.OrganismsByRegion[organism.RegionID] = set
	}
	set[organism.ID] = struct{}{}
}

func (e *Engine) deindexOrganism(organism *Organism) {
	e.removeFromRegion(organism.RegionID, organism.ID)
}

func (e *Engine) removeFromRegion(regionID RegionID, id EntityID) {
	set := e.OrganismsByRegion[regionID]
	if set == nil {
		return
	}
	delete(set, id)
	if len(set) == 0 {
		delete(e.OrganismsByRegion, regionID)
	}
}
