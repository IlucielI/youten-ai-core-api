package constants

import (
	"testing"
)

func TestWorkspaceRole_Permissions(t *testing.T) {
	tests := []struct {
		role              WorkspaceRole
		isValid           bool
		canManageMembers  bool
		canEditContent    bool
		canManageWorkspace bool
	}{
		{
			role:              WorkspaceRoleOwner,
			isValid:           true,
			canManageMembers:  true,
			canEditContent:    true,
			canManageWorkspace: true,
		},
		{
			role:              WorkspaceRoleAdmin,
			isValid:           true,
			canManageMembers:  true,
			canEditContent:    true,
			canManageWorkspace: false,
		},
		{
			role:              WorkspaceRoleMember,
			isValid:           true,
			canManageMembers:  false,
			canEditContent:    true,
			canManageWorkspace: false,
		},
		{
			role:              WorkspaceRoleViewer,
			isValid:           true,
			canManageMembers:  false,
			canEditContent:    false,
			canManageWorkspace: false,
		},
		{
			role:              WorkspaceRole("invalid"),
			isValid:           false,
			canManageMembers:  false,
			canEditContent:    false,
			canManageWorkspace: false,
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			if tc.role.IsValid() != tc.isValid {
				t.Errorf("expected IsValid() = %v for role %s", tc.isValid, tc.role)
			}
			if tc.role.CanManageMembers() != tc.canManageMembers {
				t.Errorf("expected CanManageMembers() = %v for role %s", tc.canManageMembers, tc.role)
			}
			if tc.role.CanEditContent() != tc.canEditContent {
				t.Errorf("expected CanEditContent() = %v for role %s", tc.canEditContent, tc.role)
			}
			if tc.role.CanManageWorkspace() != tc.canManageWorkspace {
				t.Errorf("expected CanManageWorkspace() = %v for role %s", tc.canManageWorkspace, tc.role)
			}
		})
	}
}

