package auth

import "strings"

// AccessClass classifies ONVIF service operations per ONVIF Core §5.9.4.3.
type AccessClass int

const (
	// ClassPreAuth means the service shall not require user authentication.
	ClassPreAuth AccessClass = iota
	// ClassReadSystem reads system configuration information.
	ClassReadSystem
	// ClassReadSystemSensitive reads sensitive (but not confidential) system info.
	ClassReadSystemSensitive
	// ClassReadSystemSecret reads confidential system info.
	ClassReadSystemSecret
	// ClassWriteSystem changes system configuration.
	ClassWriteSystem
	// ClassUnrecoverable describes irreversible configuration changes (e.g. factory reset).
	ClassUnrecoverable
	// ClassReadMedia reads recorded media.
	ClassReadMedia
	// ClassActuate affects runtime behavior (e.g. PTZ, recording jobs).
	ClassActuate
)

// Level is the ONVIF user level hierarchy (§5.9.4.2).
type Level int

const (
	// LevelAnonymous is the unauthenticated level.
	LevelAnonymous Level = iota
	// LevelUser grants read-only access.
	LevelUser
	// LevelOperator can actuate runtime behavior.
	LevelOperator
	// LevelAdministrator has full access.
	LevelAdministrator
	// LevelExtended is authorized based on external role configuration.
	LevelExtended
)

// Role name constants mirror ONVIF Core §5.9.4.2 and §5.9.4.5.
const (
	RoleAdministrator      = "Administrator"
	RoleOperator           = "Operator"
	RoleUser               = "User"
	RoleExtended           = "Extended"
	OnvifRoleAdministrator = "onvif:Administrator"
	OnvifRoleOperator      = "onvif:Operator"
	OnvifRoleUser          = "onvif:User"
)

// RoleLevel returns the user level that corresponds to role.
// Unknown roles fall back to LevelExtended, which is treated case-by-case
// by the Policy implementation.
func RoleLevel(role string) Level {
	switch strings.TrimSpace(role) {
	case RoleAdministrator, OnvifRoleAdministrator:
		return LevelAdministrator
	case RoleOperator, OnvifRoleOperator:
		return LevelOperator
	case RoleUser, OnvifRoleUser:
		return LevelUser
	case "":
		return LevelAnonymous
	default:
		return LevelExtended
	}
}

// Policy decides whether a Principal may invoke a request of the given class.
type Policy interface {
	Allow(p *Principal, class AccessClass) bool
}

// DefaultPolicy returns the access policy from ONVIF Core §5.9.4.4 Table 7.
// Principals with LevelExtended are treated as Level derived from their first
// known role, defaulting to LevelUser so Extended users never exceed their
// explicit role grants.
func DefaultPolicy() Policy {
	return defaultPolicy{}
}

type defaultPolicy struct{}

func (defaultPolicy) Allow(p *Principal, class AccessClass) bool {
	if class == ClassPreAuth {
		return true
	}
	level := principalLevel(p)
	return tableAllow(level, class)
}

func principalLevel(p *Principal) Level {
	if p == nil {
		return LevelAnonymous
	}
	best := LevelAnonymous
	for _, role := range p.Roles {
		if l := RoleLevel(role); l > best && l != LevelExtended {
			best = l
		}
	}
	return best
}

// tableAllow encodes Table 7. Administrator gets every class; other levels
// have a per-class list.
func tableAllow(level Level, class AccessClass) bool {
	if level == LevelAdministrator {
		return true
	}
	switch class {
	case ClassPreAuth:
		return true
	case ClassReadSystem, ClassReadMedia:
		return level >= LevelUser
	case ClassReadSystemSensitive, ClassActuate:
		return level >= LevelOperator
	case ClassReadSystemSecret, ClassWriteSystem, ClassUnrecoverable:
		return false // Administrator-only; already handled above.
	default:
		return false
	}
}
