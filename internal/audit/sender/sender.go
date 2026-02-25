package sender

import "github.com/heavydash/my-url-shortenergo/internal/audit"

type Sender interface {
	Name() string
	Send(event *audit.Event) error
}
