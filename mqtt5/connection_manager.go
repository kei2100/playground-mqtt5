package mqtt5

import (
	"context"
	"fmt"
	"github.com/eclipse/paho.golang/autopaho"
	"sync"
)

type connectionManager struct {
	config      *autopaho.ClientConfig
	shutdownCtx context.Context
	shutdown    context.CancelFunc

	mu      sync.RWMutex
	connMgr *autopaho.ConnectionManager
}

func newConnectionManager(config *autopaho.ClientConfig) *connectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &connectionManager{
		config:      config,
		shutdownCtx: ctx,
		shutdown:    cancel,
	}
}

func (m *connectionManager) AwaitConnectionManager(ctx context.Context) (*autopaho.ConnectionManager, error) {
	m.mu.RLock()
	if m.connMgr != nil {
		m.mu.RUnlock()
		return m.connMgr, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	// double-checked locking pattern
	if m.connMgr != nil {
		return m.connMgr, nil
	}

	connMgr, err := autopaho.NewConnection(m.shutdownCtx, *m.config)
	if err != nil {
		return nil, fmt.Errorf("mqtt5: unexpected error occurred while waiting new connection: %w", err)
	}

	ctx, cancel := context.WithCancel(m.shutdownCtx)
	defer cancel()

	if err := connMgr.AwaitConnection(ctx); err != nil {
		return nil, fmt.Errorf("mqtt5: await connection: %w", err)
	}
	m.connMgr = connMgr
	return m.connMgr, nil
}

func (m *connectionManager) Close(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	defer m.shutdown()

	if m.connMgr == nil {
		return nil
	}
	if err := m.connMgr.Disconnect(ctx); err != nil {
		return fmt.Errorf("mqtt5: disconnect: %w", err)
	}
	return nil
}
