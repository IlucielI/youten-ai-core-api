package repositories

import (
	"context"
	"encoding/json"
	"sync"
)

// RedisKeySystemConfig is the key used for persisting feature flags in Redis.
const RedisKeySystemConfig = "system:config:flags"

// SystemConfigData represents internal system configuration state.
type SystemConfigData struct {
	MaintenanceMode    bool `json:"maintenance_mode"`
	AllowGuestUploads  bool `json:"allow_guest_uploads"`
	BotWaitlistEnabled bool `json:"bot_waitlist_enabled"`
}

var (
	defaultConfig = SystemConfigData{
		MaintenanceMode:    false,
		AllowGuestUploads:  true,
		BotWaitlistEnabled: true,
	}
	fallbackConfigMu sync.RWMutex
	fallbackConfig   = defaultConfig
)

// ResetFallbackConfig resets the memory fallback state (used in tests).
func ResetFallbackConfig() {
	fallbackConfigMu.Lock()
	defer fallbackConfigMu.Unlock()
	fallbackConfig = defaultConfig
}

// GetSystemConfig retrieves active system feature flags and maintenance settings.
func (r *Repositories) GetSystemConfig(ctx context.Context) (SystemConfigData, error) {
	if r == nil || r.rdb == nil {
		fallbackConfigMu.RLock()
		defer fallbackConfigMu.RUnlock()
		return fallbackConfig, nil
	}

	val, err := r.rdb.Get(ctx, RedisKeySystemConfig)
	if err != nil {
		fallbackConfigMu.RLock()
		defer fallbackConfigMu.RUnlock()
		return fallbackConfig, nil
	}

	var data SystemConfigData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		fallbackConfigMu.RLock()
		defer fallbackConfigMu.RUnlock()
		return fallbackConfig, nil
	}

	return data, nil
}

// UpdateSystemConfig updates feature flags in Redis with fallback store.
func (r *Repositories) UpdateSystemConfig(ctx context.Context, maintenanceMode, allowGuestUploads, botWaitlistEnabled *bool) (SystemConfigData, error) {
	current, _ := r.GetSystemConfig(ctx)

	if maintenanceMode != nil {
		current.MaintenanceMode = *maintenanceMode
	}
	if allowGuestUploads != nil {
		current.AllowGuestUploads = *allowGuestUploads
	}
	if botWaitlistEnabled != nil {
		current.BotWaitlistEnabled = *botWaitlistEnabled
	}

	fallbackConfigMu.Lock()
	fallbackConfig = current
	fallbackConfigMu.Unlock()

	if r != nil && r.rdb != nil {
		payload, err := json.Marshal(current)
		if err == nil {
			_ = r.rdb.Set(ctx, RedisKeySystemConfig, string(payload), 0)
		}
	}

	return current, nil
}

// IsMaintenanceMode returns true if the system maintenance flag is active.
func (r *Repositories) IsMaintenanceMode(ctx context.Context) bool {
	cfg, _ := r.GetSystemConfig(ctx)
	return cfg.MaintenanceMode
}
