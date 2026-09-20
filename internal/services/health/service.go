package health

import "context"

type service struct {
	mongo   Pinger
	redis   Pinger
	version string
}

// New constructs a health Service.
func New(mongo, redis Pinger, version string) Service {
	return &service{
		mongo:   mongo,
		redis:   redis,
		version: version,
	}
}

func (s *service) Check(ctx context.Context) Status {
	out := Status{
		Status:  "ok",
		Mongo:   "ok",
		Redis:   "ok",
		Version: s.version,
	}

	if s.mongo != nil {
		if err := s.mongo.Ping(ctx); err != nil {
			out.Mongo = "error"
			out.Status = "degraded"
		}
	}
	if s.redis != nil {
		if err := s.redis.Ping(ctx); err != nil {
			out.Redis = "error"
			out.Status = "degraded"
		}
	}
	return out
}
