package auth

import (
	"context"
	"fmt"
	"net/http"
)

// OperationAuthorizer is the generic hook shape used by all ONVIF service
// handlers. Each service adapts this to its own typed hook with a one-line
// wrapper so the service packages remain independent of the auth package's
// types.
//
//	hook := devicesvc.AuthFunc(auth.NewOperationAuthorizer(...))
type OperationAuthorizer func(ctx context.Context, operation string, r *http.Request) error

// OperationClassFunc returns the AccessClass for an operation name.
// NewOperationAuthorizer accepts either a map[string]AccessClass (via
// MapOperationClass) or a custom resolver.
type OperationClassFunc func(operation string) AccessClass

// MapOperationClass adapts a lookup table into an OperationClassFunc.
// Unlisted operations default to ClassWriteSystem (admin-only).
func MapOperationClass(m map[string]AccessClass) OperationClassFunc {
	return func(op string) AccessClass {
		if c, ok := m[op]; ok {
			return c
		}
		return ClassWriteSystem
	}
}

// NewOperationAuthorizer returns a hook that authenticates each request and
// checks the resulting Principal against the policy for the operation's
// access class.
func NewOperationAuthorizer(a Authenticator, p Policy, classify OperationClassFunc) OperationAuthorizer {
	if a == nil || p == nil || classify == nil {
		panic("auth: NewOperationAuthorizer requires authenticator, policy, and classifier")
	}
	return func(ctx context.Context, operation string, r *http.Request) error {
		class := classify(operation)
		if class == ClassPreAuth {
			return nil
		}
		principal, err := a.Authenticate(ctx, r)
		if err != nil {
			return err
		}
		if !p.Allow(principal, class) {
			return fmt.Errorf("%w: %s not allowed for class %d", ErrForbidden, principal.Username, class)
		}
		return nil
	}
}
