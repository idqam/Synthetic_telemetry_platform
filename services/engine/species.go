package engine

import (
	"errors"

	"github.com/google/uuid"
)

type SpeciesID string

type Species struct {
	ID   SpeciesID
	Name string

	MaxEnergy       float64
	InitialEnergy   float64
	MetabolismRate  float64
	MovementCost    float64
	MaxMovementRate float64

	MaxAge float64

	Reproduction ReproductionTraits
	Feeding      FeedingTraits
	Telemetry    TelemetryTraits

	DiseaseSusceptibility float64
}

type FeedingTraits struct {
	ResourceEfficiency      map[ResourceType]float64
	MaxConsumptionPerAction float64
}

type ReproductionTraits struct {
	MinimumAge      float64
	EnergyThreshold float64
	EnergyCost      float64
	MinimumInterval float64
}

type TelemetryTraits struct {
	Cadence float64
}

func NewSpecies(name string) *Species {
	return &Species{
		ID:   SpeciesID(uuid.NewString()),
		Name: name,

		MaxEnergy:       100,
		InitialEnergy:   75,
		MetabolismRate:  1,
		MovementCost:    0.2,
		MaxMovementRate: 5,
		MaxAge:          1_000,

		Reproduction: ReproductionTraits{
			MinimumAge:      20,
			EnergyThreshold: 70,
			EnergyCost:      25,
			MinimumInterval: 50,
		},
		Feeding: FeedingTraits{
			ResourceEfficiency: map[ResourceType]float64{
				ResourceFood: 1,
			},
			MaxConsumptionPerAction: 10,
		},
		Telemetry: TelemetryTraits{
			Cadence: 1,
		},

		DiseaseSusceptibility: 0.5,
	}
}

func (s *Species) Validate() error {
	if s == nil {
		return errors.New("species cannot be nil")
	}
	if s.ID == "" {
		return errors.New("species ID cannot be empty")
	}
	if s.Name == "" {
		return errors.New("species name cannot be empty")
	}
	if s.MaxEnergy <= 0 {
		return errors.New("maximum energy must be positive")
	}
	if s.InitialEnergy < 0 || s.InitialEnergy > s.MaxEnergy {
		return errors.New("initial energy must be within species energy bounds")
	}
	if s.MetabolismRate < 0 {
		return errors.New("metabolism rate cannot be negative")
	}
	if s.MovementCost < 0 || s.MaxMovementRate < 0 {
		return errors.New("movement traits cannot be negative")
	}
	if s.MaxAge <= 0 {
		return errors.New("maximum age must be positive")
	}
	if !normalized(s.DiseaseSusceptibility) {
		return errors.New("disease susceptibility must be between 0 and 1")
	}
	if s.Telemetry.Cadence <= 0 {
		return errors.New("telemetry cadence must be positive")
	}
	return nil
}
