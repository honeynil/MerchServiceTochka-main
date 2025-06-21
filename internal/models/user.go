package models

import "time"

type User struct {
	ID           int32
	Balance      int32
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}
