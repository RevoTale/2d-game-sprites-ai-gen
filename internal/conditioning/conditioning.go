// Package conditioning defines provider-neutral image input roles.
package conditioning

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
	Role        Role
	Path        string
	Description string
	Required    bool
	SHA256      string
}
