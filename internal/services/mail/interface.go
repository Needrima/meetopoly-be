package mail

import "context"

// Message is an outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Service sends email (Google SMTP).
type Service interface {
	Send(ctx context.Context, msg Message) error
}
