package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRoleType int

const (
	Nobody UserRoleType = iota
	Registered
	Service
	Administrator
)

type User struct {
	UID          uuid.UUID
	UserName     string //userna
	UserLogin    string // email
	UserPassword string // hash
	UserRole     UserRoleType
}

type UserProfile struct {
	UID           uuid.UUID // UUID пользователя
	Plan          uuid.UUID // PlanStatus.UID -  тариф
	PlanValidDate time.Time
	Tg            string // uname telegrram
	TgId          uint64 // id telegram

	Created time.Time
}

type PlanStatus struct {
	UID uuid.UUID
}
