package utilst

import "github.com/gofiber/fiber/v3/middleware/session"

func NewTestStore() *session.Store { _, store := session.NewWithStore(); return store }
