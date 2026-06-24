package engine

import (
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
)

type DiseaseID string

type OrganismState uint8

const (
	OrganismStateAlive OrganismState = iota
	OrganismStateDead
)

func (s OrganismState) Valid() bool {
	return s == OrganismStateAlive || s == OrganismStateDead
}

type DeathCause string

const (
	DeathCauseStarvation DeathCause = "starvation"
	DeathCauseOldAge     DeathCause = "old_age"
	DeathCausePredation  DeathCause = "predation"
	DeathCauseDisease    DeathCause = "disease"
	DeathCauseOperator   DeathCause = "operator"
)

type Organism struct {
	ID        EntityID
	SpeciesID SpeciesID

	RegionID RegionID
	Position Point

	State      OrganismState
	DeathCause DeathCause

	BornAtTick    uint64
	BornAtSimTime float64
	DiedAtTick    *uint64

	Energy EnergyLevel
	Vitals Vitals

	Lineage Lineage

	Telemetry TelemetryState

	Infections map[DiseaseID]*Infection

	// Monotonic aggregate version. Useful for event ordering, optimistic concurrency, and deterministic replay.
	Version uint64
}

type EnergyLevel struct {
	Current float64
	Max     float64
}

func NewEnergyLevel(current, max float64) (EnergyLevel, error) {
	if !finite(max) || max <= 0 {
		return EnergyLevel{}, errors.New("maximum energy must be finite and positive")
	}
	if !finite(current) || current < 0 || current > max {
		return EnergyLevel{}, fmt.Errorf(
			"energy must be between 0 and %.2f; got %.2f",
			max,
			current,
		)
	}

	return EnergyLevel{
		Current: current,
		Max:     max,
	}, nil
}

func (e EnergyLevel) Ratio() float64 {
	if e.Max <= 0 {
		return 0
	}
	return e.Current / e.Max
}

func (e EnergyLevel) Depleted() bool {
	return e.Current <= epsilon
}

type Vitals struct {
	Health float64
	Stress float64
}

func (v Vitals) Validate() error {
	if !normalized(v.Health) {
		return fmt.Errorf("health must be between 0 and 1: %v", v.Health)
	}
	if !normalized(v.Stress) {
		return fmt.Errorf("stress must be between 0 and 1: %v", v.Stress)
	}
	return nil
}

type Lineage struct {
	ParentIDs  []EntityID
	Generation uint64
}

type TelemetryState struct {
	Enabled bool

	Sequence          uint64
	LastEmittedAtTick uint64
	LastEmittedAtTime float64
}

type Infection struct {
	DiseaseID DiseaseID

	ContractedAt float64
	Severity     float64
	Contagious   bool
}

func NewOrganism(
	species *Species,
	regionID RegionID,
	position Point,
	bornAtTick uint64,
	bornAtSimTime float64,
	parentIDs []EntityID,
	generation uint64,
) (*Organism, error) {
	if species == nil {
		return nil, errors.New("species cannot be nil")
	}
	if err := species.Validate(); err != nil {
		return nil, fmt.Errorf("invalid species: %w", err)
	}
	if regionID == "" {
		return nil, errors.New("region ID cannot be empty")
	}
	if !finite(position.X) || !finite(position.Y) {
		return nil, errors.New("position must contain finite coordinates")
	}
	if !finite(bornAtSimTime) || bornAtSimTime < 0 {
		return nil, errors.New("birth simulation time cannot be negative")
	}

	energy, err := NewEnergyLevel(
		species.InitialEnergy,
		species.MaxEnergy,
	)
	if err != nil {
		return nil, err
	}

	organism := &Organism{
		ID:            EntityID(uuid.NewString()),
		SpeciesID:     species.ID,
		RegionID:      regionID,
		Position:      position,
		State:         OrganismStateAlive,
		BornAtTick:    bornAtTick,
		BornAtSimTime: bornAtSimTime,
		Energy:        energy,
		Vitals: Vitals{
			Health: 1,
			Stress: 0,
		},
		Lineage: Lineage{
			ParentIDs:  append([]EntityID(nil), parentIDs...),
			Generation: generation,
		},
		Telemetry: TelemetryState{
			Enabled: true,
		},
		Infections: make(map[DiseaseID]*Infection),
		Version:    1,
	}

	return organism, organism.Validate(species)
}

func (o *Organism) Validate(species *Species) error {
	if o == nil {
		return errors.New("organism cannot be nil")
	}
	if species == nil || o.SpeciesID != species.ID {
		return errors.New("organism species does not match supplied species")
	}
	if o.ID == "" {
		return errors.New("organism ID cannot be empty")
	}
	if o.RegionID == "" {
		return errors.New("organism region ID cannot be empty")
	}
	if !o.State.Valid() {
		return fmt.Errorf("invalid organism state: %d", o.State)
	}
	if o.Energy.Max != species.MaxEnergy {
		return errors.New("organism maximum energy does not match species")
	}
	if o.Energy.Current < 0 || o.Energy.Current > o.Energy.Max {
		return errors.New("organism energy is outside valid bounds")
	}
	if err := o.Vitals.Validate(); err != nil {
		return err
	}

	if o.State == OrganismStateAlive {
		if o.DeathCause != "" || o.DiedAtTick != nil {
			return errors.New("living organism cannot have death information")
		}
	}

	if o.State == OrganismStateDead {
		if o.DeathCause == "" || o.DiedAtTick == nil {
			return errors.New("dead organism requires death cause and tick")
		}
	}

	return nil
}

func normalized(value float64) bool {
	return finite(value) && value >= 0 && value <= 1
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
