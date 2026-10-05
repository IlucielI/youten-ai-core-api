package services

import (
	"context"
	"time"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// CheckHealth inspects the connectivity of sub-systems (database, redis, s3 storage).
func (s *Service) CheckHealth(ctx context.Context) dtos.HealthServices {
	dbStatus := constants.IntegrationStatusConnected
	if s.repo == nil {
		dbStatus = constants.IntegrationStatusDisconnected
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.repo.PingDB(pingCtx); err != nil {
			dbStatus = constants.IntegrationStatusDisconnected
		}
	}

	redisStatus := constants.IntegrationStatusConnected
	if s.repo == nil {
		redisStatus = constants.IntegrationStatusDisconnected
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.repo.PingRedis(pingCtx); err != nil {
			redisStatus = constants.IntegrationStatusDisconnected
		}
	}

	s3Status := constants.IntegrationStatusConnected
	if s.storage == nil {
		s3Status = constants.IntegrationStatusDisconnected
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.storage.Ping(pingCtx); err != nil {
			s3Status = constants.IntegrationStatusDisconnected
		}
	}

	rmqStatus := constants.IntegrationStatusConnected
	if s.publisher == nil {
		rmqStatus = constants.IntegrationStatusDisconnected
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.publisher.Ping(pingCtx); err != nil {
			rmqStatus = constants.IntegrationStatusDisconnected
		}
	}

	smtpStatus := constants.IntegrationStatusConnected
	if s.mailer == nil {
		smtpStatus = constants.IntegrationStatusDisconnected
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.mailer.Ping(pingCtx); err != nil {
			smtpStatus = constants.IntegrationStatusDisconnected
		}
	}

	return dtos.HealthServices{
		Database: dbStatus,
		Redis:    redisStatus,
		S3:       s3Status,
		RabbitMQ: rmqStatus,
		SMTP:     smtpStatus,
	}
}
