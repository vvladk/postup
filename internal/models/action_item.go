package models

import "time"

type ActionItem struct {
	ID         int64
	RetroID    int64
	ColumnID   int64
	AssigneeID int64
	Content    string
	Deadline   time.Time
	StatusID   int64
	CreatedAt  time.Time
}

type ActionItemWithDetails struct {
	ActionItem
	AssigneeFirstName string
	AssigneeLastName  string
	StatusName        string
	StatusColor       string
}
