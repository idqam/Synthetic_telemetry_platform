package engine

import "time"

type EventName string

const (
	OrganismBorn          EventName = "organism.born"
	OrganismMoved         EventName = "organism.moved"
	OrganismDied          EventName = "organism.died"
	OrganismFed           EventName = "organism.fed"
	OrganismTelemetryTick EventName = "organism.telemetry"
	OrganismEnergyChanged EventName = "organism.energy_changed"
)

type Event struct {
	ID        EntityID // unique per event, for idempotent consumers
	Name      EventName
	Tick      uint64
	SimTime   float64
	Timestamp time.Time
	EntityID  EntityID // partition key (organism or region)
	Version   uint64   // per-entity monotonic sequence from Organism.Version
	Payload   any
}

type BornPayload struct {
	SpeciesID  SpeciesID
	RegionID   RegionID
	Position   Point
	ParentIDs  []EntityID
	Generation uint64
	Energy     float64
}

type DiedPayload struct {
	Cause       DeathCause
	RegionID    RegionID
	Position    Point
	FinalEnergy float64
	Age         float64
}

type MovedPayload struct {
	FromRegionID RegionID
	FromPosition Point
	ToRegionID   RegionID
	ToPosition   Point
	EnergyCost   float64
}

type FedPayload struct {
	RegionID     RegionID
	Resource     ResourceType
	Consumed     float64
	EnergyGained float64
	EnergyAfter  float64
}

type EnergyChangedPayload struct {
	Reason       string
	EnergyBefore float64
	EnergyAfter  float64
}

type TelemetryPayload struct {
	SpeciesID SpeciesID
	RegionID  RegionID
	Position  Point
	Energy    EnergyLevel
	Vitals    Vitals
	Age       float64
}
