package mqtt5

import (
	"context"
	"fmt"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"sync"
)

type subscriptionManager struct {
	subscriptions []*paho.Subscribe
	mu            sync.Mutex
}

func newSubscriptionManager() *subscriptionManager {
	return &subscriptionManager{}
}

func (m *subscriptionManager) Subscribe(ctx context.Context, connMgr *autopaho.ConnectionManager, subscribe *paho.Subscribe) error {
	if _, err := connMgr.Subscribe(ctx, subscribe); err != nil {
		return fmt.Errorf("mqtt5: send subscribe: %w", err)
	}

	m.mu.Lock()
	m.subscriptions = append(m.subscriptions, subscribe)
	m.mu.Unlock()

	return nil
}

func (m *subscriptionManager) ReSubscribe(ctx context.Context, connMgr *autopaho.ConnectionManager) error {
	m.mu.Lock()
	subs := m.subscriptions
	m.mu.Unlock()

	for _, s := range subs {
		if _, err := connMgr.Subscribe(ctx, s); err != nil {
			return fmt.Errorf("mqtt5: send subscribe: %w", err)
		}
	}
	return nil
}
