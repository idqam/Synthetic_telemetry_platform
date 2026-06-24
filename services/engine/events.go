package engine

type EventName string

const (
	OrganismBorn EventName = "organism.born"
	OrganismMoved EventName = "organism.moved"
	OrganismTelemetryTick EventName = "organism.telemetry"
)
