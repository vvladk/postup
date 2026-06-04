package models

import "time"

type Template struct {
	ID          int64
	Name        string
	Status      string
	CreatedAt   time.Time
	RetrosCount int
	Columns     []TemplateColumn
}

func (t *Template) HasRetros() bool {
	return t.RetrosCount > 0
}

func (t *Template) IsArchived() bool {
	return t.Status == "archived"
}

type TemplateColumn struct {
	ID         int64
	TemplateID int64
	Title      string
	Position   int
	Type       string
}
