package models

import (
	"fmt"
	"time"
)

type User struct {
	ID           int32     `json:"id"`
	Balance      int32     `json:"balance"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

func (u *User) Validate() error {
	if u.Username == "" {
		return fmt.Errorf("username is required")
	}
	if len(u.Username) > 50 {
		return fmt.Errorf("username too long")
	}
	if u.PasswordHash == "" {
		return fmt.Errorf("password_hash is required")
	}
	return nil
}
