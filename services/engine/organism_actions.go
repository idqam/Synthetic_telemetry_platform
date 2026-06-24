package engine

import (
	"errors"
	"fmt"
	"math"
)

var ErrOrganismDead = errors.New("organism is dead")

func (o *Organism) Alive() bool {
	return o != nil && o.State == OrganismStateAlive
}

func (o *Organism) AgeAt(simTime float64) float64 {
	if o == nil || simTime <= o.BornAtSimTime {
		return 0
	}
	return simTime - o.BornAtSimTime
}

func (o *Organism) SpendEnergy(amount float64) error {
	if !o.Alive() {
		return ErrOrganismDead
	}
	if !finite(amount) || amount < 0 {
		return errors.New("energy amount must be finite and non-negative")
	}
	if amount > o.Energy.Current+epsilon {
		return ErrInsufficientResource
	}

	o.Energy.Current = math.Max(0, o.Energy.Current-amount)
	o.Version++
	return nil
}

func (o *Organism) RestoreEnergy(amount float64) float64 {
	if !o.Alive() || !finite(amount) || amount <= 0 {
		return 0
	}

	before := o.Energy.Current
	o.Energy.Current = math.Min(o.Energy.Max, o.Energy.Current+amount)

	restored := o.Energy.Current - before
	if restored > 0 {
		o.Version++
	}

	return restored
}

// ApplyCrowdingStress sets the organism's stress from region occupancy. Crowding
// raises stress proportional to how full the region is; it never kills.
func (o *Organism) ApplyCrowdingStress(ratio float64) {
	if !o.Alive() {
		return
	}
	o.Vitals.Stress = clamp(ratio, 0, 1)
	o.Version++
}

func (o *Organism) ApplyMetabolism(
	delta float64,
	species *Species,
) error {
	if !o.Alive() {
		return ErrOrganismDead
	}
	if species == nil || species.ID != o.SpeciesID {
		return errors.New("incorrect species supplied")
	}
	if !finite(delta) || delta <= 0 {
		return errors.New("delta must be finite and positive")
	}

	cost := species.MetabolismRate * delta
	o.Energy.Current = math.Max(0, o.Energy.Current-cost)
	o.Version++

	return nil
}

func (o *Organism) ApplyMovement(
	destination Point,
	destinationRegionID RegionID,
	species *Species,
) error {
	if !o.Alive() {
		return ErrOrganismDead
	}
	if destinationRegionID == "" {
		return errors.New("destination region cannot be empty")
	}

	distance := distanceBetween(o.Position, destination)
	if distance > species.MaxMovementRate+epsilon {
		return fmt.Errorf(
			"movement distance %.2f exceeds species limit %.2f",
			distance,
			species.MaxMovementRate,
		)
	}

	energyCost := distance * species.MovementCost
	if err := o.SpendEnergy(energyCost); err != nil {
		return fmt.Errorf("movement energy: %w", err)
	}

	o.Position = destination
	o.RegionID = destinationRegionID
	o.Version++
	return nil
}

func (o *Organism) EnergyFromResource(
	resource ResourceType,
	amount float64,
	species *Species,
) (float64, error) {
	if !o.Alive() {
		return 0, ErrOrganismDead
	}
	if amount < 0 {
		return 0, errors.New("resource amount cannot be negative")
	}

	efficiency, edible := species.Feeding.ResourceEfficiency[resource]
	if !edible || efficiency <= 0 {
		return 0, fmt.Errorf(
			"species %q cannot consume resource %q",
			species.ID,
			resource,
		)
	}

	return amount * efficiency, nil
}

func FeedOrganism(
	organism *Organism,
	species *Species,
	region *Region,
	resource ResourceType,
	requestedAmount float64,
) (consumed float64, energyGained float64, err error) {
	if organism.RegionID != region.ID {
		return 0, 0, errors.New("organism is not in the supplied region")
	}

	maxAllowed := math.Min(
		requestedAmount,
		species.Feeding.MaxConsumptionPerAction,
	)

	consumed = region.Resources.RemoveUpTo(resource, maxAllowed)

	energyGained, err = organism.EnergyFromResource(
		resource,
		consumed,
		species,
	)
	if err != nil {
		region.Resources.Add(resource, consumed)
		return 0, 0, err
	}

	energyGained = organism.RestoreEnergy(energyGained)
	return consumed, energyGained, nil
}

func (o *Organism) Die(cause DeathCause, tick uint64) error {
	if !o.Alive() {
		return ErrOrganismDead
	}
	if cause == "" {
		return errors.New("death cause cannot be empty")
	}

	o.State = OrganismStateDead
	o.DeathCause = cause
	o.DiedAtTick = &tick
	o.Telemetry.Enabled = false
	o.Version++

	return nil
}

func (o *Organism) ShouldEmitTelemetry(
	simTime float64,
	species *Species,
) bool {
	if !o.Alive() || !o.Telemetry.Enabled {
		return false
	}

	return simTime-o.Telemetry.LastEmittedAtTime >=
		species.Telemetry.Cadence-epsilon
}

func (o *Organism) RecordTelemetryEmission(
	tick uint64,
	simTime float64,
) {
	o.Telemetry.Sequence++
	o.Telemetry.LastEmittedAtTick = tick
	o.Telemetry.LastEmittedAtTime = simTime
}

func (o *Organism) Advance(
	tick uint64,
	simTime float64,
	delta float64,
	species *Species,
) error {
	if !o.Alive() {
		return nil
	}

	if err := o.ApplyMetabolism(delta, species); err != nil {
		return err
	}

	switch {
	case o.Energy.Depleted():
		return o.Die(DeathCauseStarvation, tick)

	case o.AgeAt(simTime) >= species.MaxAge:
		return o.Die(DeathCauseOldAge, tick)
	}

	return nil
}

func distanceBetween(a, b Point) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}
