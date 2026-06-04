package models

import "time"

type User struct {
	ID                  int64
	Email               string
	PasswordHash        string
	FirstName           string
	LastName            string
	Role                string
	InviteToken         *string
	InviteUsedAt        *time.Time
	ResetToken          *string
	ResetTokenExpiresAt *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}
