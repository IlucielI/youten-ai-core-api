package constants

// Administrative Audit Entities
const (
	AuditEntityAdminUser    = "ADMIN_USER"
	AuditEntityUser         = "user"
	AuditEntityRole         = "role"
	AuditEntityUserRole     = "user_role"
	AuditEntityTemplate     = "template"
	AuditEntityRecording    = "recording"
	AuditEntitySystemConfig = "system_config"
	AuditEntityReport       = "report"
)

// Administrative Audit Actions
const (
	AuditActionAdminLogin         = "ADMIN_LOGIN"
	AuditActionAdminPasswordReset = "ADMIN_PASSWORD_RESET"
	AuditActionUserQuotaOverride  = "user.quota_override"
	AuditActionUserRevokeSessions = "user.revoke_sessions"
	AuditActionUserAssignRole     = "user.assign_role"
	AuditActionRoleCreate         = "role.create"
	AuditActionRoleUpdate         = "role.update"
	AuditActionUserRoleCreate     = "user_role.create"
	AuditActionUserRoleUpdate     = "user_role.update"
	AuditActionTemplateCreate     = "template.create"
	AuditActionTemplateUpdate     = "template.update"
	AuditActionPipelineDLQRetry   = "pipeline.dlq_retry"
	AuditActionConfigUpdateFlags  = "config.update_flags"
	AuditActionReportResolve      = "report.resolve"
)
