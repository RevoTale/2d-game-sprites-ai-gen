// Package conditioning defines provider-neutral image input roles.
package conditioning

import "fmt"

// Role describes how a generation input should influence an output.
type Role uint8

const (
	RoleStyle Role = iota + 1
	RoleIdentity
	RolePose
	RoleMask
)

// Input preserves the source and purpose of one generation image.
type Input struct {
	ID          string
	Role        Role
	Authority   string
	SourcePath  string
	Path        string
	Description string
	Required    bool
	SHA256      string
}

func (role Role) String() string {
	switch role {
	case RoleStyle:
		return "style"
	case RoleIdentity:
		return "identity"
	case RolePose:
		return "motion"
	case RoleMask:
		return "mask"
	default:
		return fmt.Sprintf("unknown-%d", role)
	}
}
