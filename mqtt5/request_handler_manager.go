package mqtt5

import (
	"context"
	"github.com/eclipse/paho.golang/autopaho/extensions/rpc"
	"sync"
)

// TODO test
type requestHandlerManager struct {
	mu     sync.RWMutex
	rh     *rpc.Handler
	waitCh chan struct{}
}

func newRequestHandlerManager() *requestHandlerManager {
	return &requestHandlerManager{
		waitCh: make(chan struct{}),
	}
}

func (m *requestHandlerManager) SetRequestHandler(rh *rpc.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rh = rh
	select {
	case <-m.waitCh:
		// already closed. noop
	default:
		close(m.waitCh)
	}
}

func (m *requestHandlerManager) ResetRequestHandler() {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-m.waitCh:
		// already closed. noop
	default:
		close(m.waitCh)
	}
	m.waitCh = make(chan struct{})
	m.rh = nil
}

func (m *requestHandlerManager) AwaitRequestHandler(ctx context.Context) (*rpc.Handler, error) {
	for {
		m.mu.RLock()
		ch := m.waitCh
		m.mu.RUnlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
			m.mu.RLock()
			rh := m.rh
			m.mu.RUnlock()
			if rh == nil {
				continue
			}
			return rh, nil
		}
	}
}
