package types

import "github.com/google/uuid"

type PipelineStatus struct {
	ID           string
	Name         string
	Status       string
	Result       string
	CreatedDate  string
	FinishedDate string
	Stages       []StageStatus
	TimeElapsed  string
}

type StageStatus struct {
	ID         uuid.UUID
	ApprovalID *uuid.UUID
	Name       string
	Status     string
	Result     string
	Order      int
}

type Observer interface {
	Update([]PipelineStatus)
}

type Subject interface {
	Register(Observer)
	Unregister(Observer)
	NotifyAll()
}
