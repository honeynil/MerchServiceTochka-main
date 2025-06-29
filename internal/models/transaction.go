package models

import "time"

type Transaction struct {
	ID        int32           `json:"id"`
	UserID    int32           `json:"user_id"`
	RelatedID int32           `json:"related_id,omitempty"`
	Amount    int32           `json:"amount"`
	Type      TransactionType `json:"type"`
	Status    StatusType      `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

type TransactionType string

const (
	TypePurchase TransactionType = "purchase"
	TypeTransfer TransactionType = "transfer"
)

type StatusType string

const (
	StatusPending   StatusType = "pending"
	StatusCompleted StatusType = "completed"
	StatusFailed    StatusType = "failed"
)
