package server

import (
	"github.com/DrmagicE/gmqtt/persistence/queue"
	"github.com/DrmagicE/gmqtt/subscription"
)

type Persistence interface {
	Open() error
	NewQueueStore(config Config, client Client) (queue.Store, error)
	NewSubscriptionStore(config Config) subscription.Store
	Close() error
}

type PersistenceFactory interface {
	New(config Config, hooks Hooks) (Persistence, error)
}
