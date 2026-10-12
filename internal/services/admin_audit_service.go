package services

import (
	"context"
	"log"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
	"code-base-golang/internal/queue"
)

// RecordAdminAuditLog processes and persists an administrative audit log from a background worker or fallback.
func (s *Service) RecordAdminAuditLog(ctx context.Context, p queue.AdminAuditPayload) error {
	var payloadMap models.JSONMap
	if p.Payload != nil {
		payloadMap = models.JSONMap(p.Payload)
	}

	return s.repo.CreateAdminAuditLog(ctx, &models.AdminAuditLog{
		AdminID:   p.AdminID,
		Action:    p.Action,
		Entity:    p.Entity,
		EntityID:  p.EntityID,
		Payload:   payloadMap,
		IPAddress: p.IPAddress,
		UserAgent: p.UserAgent,
		CreatedAt: p.CreatedAt,
	})
}

// emitAdminAuditLog dispatches an administrative audit log event to the message broker.
// If the broker publisher is nil or fails, it falls back to saving directly without breaking the calling flow.
func (s *Service) emitAdminAuditLog(ctx context.Context, p queue.AdminAuditPayload) {
	if s.publisher != nil {
		if err := s.publisher.Publish(ctx, constants.TopicAdminAuditLog, p); err != nil {
			log.Printf("[AUDIT WARN] failed to publish admin audit log to broker: %v, falling back to direct save", err)
			_ = s.RecordAdminAuditLog(ctx, p)
		}
		return
	}
	// Fallback when broker is not available (e.g. testing)
	_ = s.RecordAdminAuditLog(ctx, p)
}
