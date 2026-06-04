package models

import "time"

type Retro struct {
	ID         int64
	TeamID     int64
	TemplateID *int64
	Date       time.Time
	VoteLimit  int
	Status     string
	CreatedAt  time.Time
}

func (r *Retro) IsActive() bool {
	return r.Status == "active" && r.Date.After(time.Now().Add(-time.Hour))
}

func (r *Retro) ShouldArchive() bool {
	return r.Status == "active" && r.Date.Before(time.Now().Add(-time.Hour))
}

type RetroWithDetails struct {
	Retro
	TeamName          string
	TemplateName      string
	ParticipantsCount int
}
