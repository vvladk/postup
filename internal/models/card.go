package models

import "time"

type Card struct {
	ID        int64
	RetroID   int64
	ColumnID  int64
	AuthorID  int64
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *Card) IsOwnedBy(userID int64) bool {
	return c.AuthorID == userID
}

type CardWithAuthor struct {
	Card
	AuthorFirstName string
	AuthorLastName  string
}
