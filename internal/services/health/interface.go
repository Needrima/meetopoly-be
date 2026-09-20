package health

import "context"

// Status is the health payload returned to clients.
type Status struct {
	Status  string `json:"status"`
	Mongo   string `json:"mongo"`
	Redis   string `json:"redis"`
	Version string `json:"version,omitempty"`
}

// Pinger checks a dependency.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Service is the health use-case port.
type Service interface {
	Check(ctx context.Context) Status
}
