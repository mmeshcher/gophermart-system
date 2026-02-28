// Package model содержит доменные сущности сервиса гофермарт.
package model

import "time"

// User представляет зарегистрированного пользователя системы лояльности.
type User struct {
	ID           int64
	Login        string
	PasswordHash []byte
	CreatedAt    time.Time
}

// OrderStatus описывает статус обработки заказа.
type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "NEW"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

// Order описывает заказ пользователя и данные о начислениях.
type Order struct {
	Number     string
	Status     OrderStatus
	Accrual    *float64
	UploadedAt time.Time
}

// Withdrawal описывает факт списания средств с бонусного счёта.
type Withdrawal struct {
	Order       string
	Sum         float64
	ProcessedAt time.Time
}

// Balance содержит баланс пользователя и сумму всех списаний.
type Balance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}
