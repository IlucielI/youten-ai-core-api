package repositories

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"code-base-golang/internal/adapters/redis"
)

// Repositories is the unified data access container.
type Repositories struct {
	db  *gorm.DB
	rdb *redis.Redis
}

// New initializes the central repositories container.
func New(db *gorm.DB, rdb ...*redis.Redis) *Repositories {
	var redisAdapter *redis.Redis
	if len(rdb) > 0 {
		redisAdapter = rdb[0]
	}
	return &Repositories{
		db:  db,
		rdb: redisAdapter,
	}
}

// DB returns the underlying GORM database instance.
func (r *Repositories) DB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.db
}

// Redis returns the underlying Redis adapter instance.
func (r *Repositories) Redis() *redis.Redis {
	if r == nil {
		return nil
	}
	return r.rdb
}

// PingDB checks connectivity to Postgres via GORM's underlying sql.DB.
func (r *Repositories) PingDB(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("database connection is nil")
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// PingRedis checks connectivity to Redis.
func (r *Repositories) PingRedis(ctx context.Context) error {
	if r == nil || r.rdb == nil {
		return errors.New("redis connection is nil")
	}
	return r.rdb.Ping(ctx)
}
