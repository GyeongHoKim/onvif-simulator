package wsdiscovery

import (
	"crypto/rand"
	"fmt"
)

// NewMessageID returns a new WS-Addressing MessageID (UUID URN style used in WS-Discovery examples).
func NewMessageID() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
