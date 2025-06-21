package models

import "time"

type Transaction struct {
	ID        int32
	UserID    int32
	RelatedID int32
	Amount    int32
	Type      TransactionType
	Status    StatusType
	CreatedAt time.Time
}

type TransactionType string

const (
	TypePurchase TransactionType = "purchase"
	TypeTransfer TransactionType = "transfer"
)

type StatusType string

const (
	StatusPending   StatusType = "pending"   // Ожидает выполнения
	StatusCompleted StatusType = "completed" // Успешно завершена
	StatusFailed    StatusType = "failed"    // Не удалась
)
