package constants

// Administrative Audit Entities (consistent lowercase dot notation)
const (
	AuditEntityAdmin        = "admin"
	AuditEntityUser         = "user"
	AuditEntityRole         = "role"
	AuditEntityUserRole     = "user.role"
	AuditEntityTemplate     = "template"
	AuditEntityRecording    = "recording"
	AuditEntitySystemConfig = "system.config"
	AuditEntityReport       = "report"
)

// Administrative Audit Actions (consistent lowercase dot notation matching role.create)
const (
	AuditActionAdminLogin         = "admin.login"
	AuditActionAdminRefreshToken  = "admin.token.refresh"
	AuditActionAdminPasswordReset = "admin.password.reset"
	AuditActionUserQuotaOverride  = "user.quota.override"
	AuditActionUserRevokeSessions = "user.sessions.revoke"
	AuditActionUserAssignRole     = "user.role.assign"
	AuditActionRoleCreate         = "role.create"
	AuditActionRoleUpdate         = "role.update"
	AuditActionUserRoleCreate     = "user.role.create"
	AuditActionUserRoleUpdate     = "user.role.update"
	AuditActionTemplateCreate     = "template.create"
	AuditActionTemplateUpdate     = "template.update"
	AuditActionPipelineDLQRetry   = "pipeline.dlq.retry"
	AuditActionConfigUpdateFlags  = "config.flags.update"
	AuditActionReportResolve      = "report.resolve"
)
