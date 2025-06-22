package models

import (
	"fmt"
	"time"
)

type User struct {
	ID           int32
	Balance      int32
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

func (u *User) Validate() error {
	if u.Username == "" {
		return fmt.Errorf("username is required")
	}
	if u.PasswordHash == "" {
		return fmt.Errorf("password_hash is required")
	}
	return nil
}
