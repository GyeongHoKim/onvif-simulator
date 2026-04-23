package auth_test

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

func admin() *auth.Principal    { return &auth.Principal{Roles: []string{auth.OnvifRoleAdministrator}} }
func operator() *auth.Principal { return &auth.Principal{Roles: []string{auth.OnvifRoleOperator}} }
func user() *auth.Principal     { return &auth.Principal{Roles: []string{auth.OnvifRoleUser}} }

func TestDefaultPolicyTable7(t *testing.T) {
	t.Parallel()
	p := auth.DefaultPolicy()

	cases := []struct {
		class          auth.AccessClass
		admin, op, usr bool
	}{
		{auth.ClassPreAuth, true, true, true},
		{auth.ClassReadSystem, true, true, true},
		{auth.ClassReadSystemSensitive, true, true, false},
		{auth.ClassReadSystemSecret, true, false, false},
		{auth.ClassWriteSystem, true, false, false},
		{auth.ClassUnrecoverable, true, false, false},
		{auth.ClassReadMedia, true, true, true},
		{auth.ClassActuate, true, true, false},
	}
	for _, c := range cases {
		if got := p.Allow(admin(), c.class); got != c.admin {
			t.Errorf("Admin class %d: got %v want %v", c.class, got, c.admin)
		}
		if got := p.Allow(operator(), c.class); got != c.op {
			t.Errorf("Operator class %d: got %v want %v", c.class, got, c.op)
		}
		if got := p.Allow(user(), c.class); got != c.usr {
			t.Errorf("User class %d: got %v want %v", c.class, got, c.usr)
		}
	}
}

func TestDefaultPolicyAnonymous(t *testing.T) {
	t.Parallel()
	p := auth.DefaultPolicy()
	if !p.Allow(nil, auth.ClassPreAuth) {
		t.Fatal("anonymous should access PRE_AUTH")
	}
	if p.Allow(nil, auth.ClassReadSystem) {
		t.Fatal("anonymous should not access READ_SYSTEM")
	}
}

func TestDeviceOperationClass(t *testing.T) {
	t.Parallel()
	if auth.DeviceOperationClass("GetServices") != auth.ClassPreAuth {
		t.Fatal("GetServices must be PreAuth")
	}
	if auth.DeviceOperationClass("GetDeviceInformation") != auth.ClassReadSystem {
		t.Fatal("GetDeviceInformation must be ReadSystem")
	}
	if auth.DeviceOperationClass("SetSystemFactoryDefault") != auth.ClassUnrecoverable {
		t.Fatal("SetSystemFactoryDefault must be Unrecoverable")
	}
	// Unknown operations default to WRITE_SYSTEM (admin-only).
	if auth.DeviceOperationClass("DoesNotExist") != auth.ClassWriteSystem {
		t.Fatal("unknown op must default to WRITE_SYSTEM")
	}
}
