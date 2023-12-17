package mqtt5

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/autopaho/extensions/rpc"
	"github.com/eclipse/paho.golang/paho"
)

type Client struct {
	brokerURL   *url.URL
	config      *paho.ClientConfig
	shutdownCtx context.Context
	shutdown    context.CancelFunc

	connMgr    *autopaho.ConnectionManager
	reqHandler *rpc.Handler
	mu         sync.Mutex
}

func NewClient(brokerURL *url.URL) *Client {
	shutdownCtx, shutdown := context.WithCancel(context.Background())
	return &Client{
		brokerURL: brokerURL,
		config: &paho.ClientConfig{
			Router: paho.NewStandardRouter(),
		},
		shutdownCtx: shutdownCtx,
		shutdown:    shutdown,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connMgr != nil {
		return errors.New("mqtt5: already connected")
	}

	config := autopaho.ClientConfig{
		BrokerUrls:        []*url.URL{c.brokerURL},
		KeepAlive:         30,
		ConnectRetryDelay: time.Second,
		ConnectTimeout:    time.Second,
		OnConnectionUp: func(*autopaho.ConnectionManager, *paho.Connack) {
			fmt.Println("mqtt5: connected")
		},
		OnConnectError: func(err error) {
			fmt.Printf("mqtt5: disconnected: %+v\n", err)
		},
		ClientConfig: *c.config,
	}
	connMgr, err := autopaho.NewConnection(c.shutdownCtx, config)
	if err != nil {
		return fmt.Errorf("mqtt5: unexpected error occurred while waiting new connection: %w", err)
	}
	if err := connMgr.AwaitConnection(ctx); err != nil {
		return fmt.Errorf("mqtt5: await connection: %w", err)
	}
	c.connMgr = connMgr
	return nil
}

func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connMgr == nil {
		return errors.New("mqtt5: already connected")
	}
	if err := c.connMgr.Disconnect(ctx); err != nil {
		return fmt.Errorf("mqtt5: disconnect: %w", err)
	}
	c.connMgr = nil
	c.reqHandler = nil
	return nil
}

// TODO refine
func (c *Client) Request(ctx context.Context, topic string, payload []byte) ([]byte, error) {
	var reqHandler *rpc.Handler
	var err error
	func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.reqHandler == nil {
			c.reqHandler, err = rpc.NewHandler(ctx, rpc.HandlerOpts{
				Conn:             c.connMgr,
				Router:           c.config.Router,
				ResponseTopicFmt: "%s/responses",
				ClientID:         c.config.ClientID,
			})
			if err != nil {
				err = fmt.Errorf("mqtt5: new request handler: %w", err)
			}
		}
		reqHandler = c.reqHandler
	}()
	if err != nil {
		return nil, err
	}
	resp, err := reqHandler.Request(ctx, &paho.Publish{
		Topic:   topic,
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("mqtt5: send request: %w", err)
	}
	return resp.Payload, nil
}

// TODO refine
func (c *Client) RegisterRequestHandler(topic string, handler func(req []byte) (resp []byte)) {
	c.config.Router.RegisterHandler(topic, func(m *paho.Publish) {
		if m.Properties == nil || m.Properties.CorrelationData == nil || m.Properties.ResponseTopic == "" {
			return
		}
		resp := handler(m.Payload)

		var connMgr *autopaho.ConnectionManager
		c.mu.Lock()
		connMgr = c.connMgr
		c.mu.Unlock()

		if _, err := connMgr.Publish(context.TODO(), &paho.Publish{
			Properties: &paho.PublishProperties{
				CorrelationData: m.Properties.CorrelationData,
			},
			Topic:   m.Properties.ResponseTopic,
			Payload: resp,
		}); err != nil {
			// TODO err handle?
			fmt.Printf("publish response: %+v\n", err)
		}
	})
}

func (c *Client) Subscribe(ctx context.Context, topic string) error {
	var connMgr *autopaho.ConnectionManager
	c.mu.Lock()
	connMgr = c.connMgr
	c.mu.Unlock()

	if _, err := connMgr.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{Topic: topic},
		},
	}); err != nil {
		return fmt.Errorf("mqtt5: send subscribe: %w", err)
	}
	return nil
}
