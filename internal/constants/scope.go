package constants

// Scope defines granular permissions for client apps, anonymous sessions, and user roles.
type Scope string

const (
	// Guest & Public Scopes
	ScopeRecordingsCreate Scope = "recordings:create"
	ScopeRecordingsRead   Scope = "recordings:read"
	ScopeRecordingsChat   Scope = "recordings:chat"
	ScopeAuthClaim        Scope = "auth:claim"
	ScopeWaitlistJoin     Scope = "waitlist:join"

	// Registered User & Pro Permissions
	ScopeRecordingsShare  Scope = "recordings:share"
	ScopeRecordingsSearch Scope = "recordings:search"
	ScopeWorkspaceMemory  Scope = "workspace:memory"
	ScopeSpeakersManage   Scope = "speakers:manage"
	ScopeExportPDF        Scope = "export:pdf"
	ScopeProfileManage    Scope = "profile:manage"

	// Wildcard Scope
	ScopeWildcard Scope = "*"
)

// DefaultClientAppScopes returns the baseline scopes granted to official client applications.
func DefaultClientAppScopes() []string {
	return []string{
		string(ScopeRecordingsCreate),
		string(ScopeRecordingsRead),
		string(ScopeRecordingsChat),
		string(ScopeAuthClaim),
		string(ScopeWaitlistJoin),
	}
}
