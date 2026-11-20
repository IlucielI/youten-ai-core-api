package constants

// WorkspaceType defines whether a workspace is an individual personal space or a collaborative team space.
type WorkspaceType string

const (
	WorkspaceTypePersonal WorkspaceType = "personal"
	WorkspaceTypeTeam     WorkspaceType = "team"
)

// WorkspaceRole defines the 4-tier RBAC authorization level within a workspace.
type WorkspaceRole string

const (
	WorkspaceRoleOwner  WorkspaceRole = "owner"
	WorkspaceRoleAdmin  WorkspaceRole = "admin"
	WorkspaceRoleMember WorkspaceRole = "member"
	WorkspaceRoleViewer WorkspaceRole = "viewer"
)

// IsValid checks if the provided role is a recognized workspace role.
func (r WorkspaceRole) IsValid() bool {
	switch r {
	case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleViewer:
		return true
	default:
		return false
	}
}

// CanManageMembers returns true if the role can invite or remove members and alter roles.
func (r WorkspaceRole) CanManageMembers() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin
}

// CanEditContent returns true if the role can upload, edit recordings, or run bot sessions.
func (r WorkspaceRole) CanEditContent() bool {
	return r == WorkspaceRoleOwner || r == WorkspaceRoleAdmin || r == WorkspaceRoleMember
}

// CanManageWorkspace returns true if the role can update workspace settings or delete it.
func (r WorkspaceRole) CanManageWorkspace() bool {
	return r == WorkspaceRoleOwner
}

