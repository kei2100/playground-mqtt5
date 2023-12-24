package mqtt5

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/eclipse/paho.golang/autopaho/extensions/rpc"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

type Client struct {
	config                *paho.ClientConfig
	connectionManager     *connectionManager
	requestHandlerManager *requestHandlerManager
	subscriptionManager   *subscriptionManager
	shutdownCtx           context.Context
	shutdown              context.CancelFunc
}

func NewClient(brokerURL *url.URL) *Client {
	shutdownCtx, shutdown := context.WithCancel(context.Background())
	rhManager := newRequestHandlerManager()
	subMgr := newSubscriptionManager()
	config := &paho.ClientConfig{
		Router: paho.NewStandardRouter(),
	}
	autoConfig := &autopaho.ClientConfig{
		BrokerUrls:        []*url.URL{brokerURL},
		KeepAlive:         30,
		ConnectRetryDelay: time.Second,
		ConnectTimeout:    time.Second,
		OnConnectionUp: func(connMgr *autopaho.ConnectionManager, _ *paho.Connack) {
			fmt.Println("mqtt5: connected")
			fmt.Println("mqtt5: setup request handler")
			rh, err := awaitNewRequestHandler(shutdownCtx, connMgr, config)
			if err != nil {
				fmt.Printf("mqtt5: new request handler: %+v", err)
				return
			}
			rhManager.SetRequestHandler(rh)
			fmt.Println("mqtt5: setup subscriptions")
			if err := subMgr.ReSubscribe(shutdownCtx, connMgr); err != nil {
				fmt.Printf("mqtt5: re-subscribe: %+v", err)
				return
			}
		},
		OnConnectError: func(err error) {
			fmt.Printf("mqtt5: disconnected: %+v\n", err)
			rhManager.ResetRequestHandler()
		},
		ClientConfig: *config,
	}
	connMgr := newConnectionManager(autoConfig)
	return &Client{
		config:                config,
		connectionManager:     connMgr,
		requestHandlerManager: rhManager,
		subscriptionManager:   subMgr,
		shutdownCtx:           shutdownCtx,
		shutdown:              shutdown,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	ctx, cancel := context.WithCancel(c.shutdownCtx)
	defer cancel()

	if _, err := c.connectionManager.AwaitConnectionManager(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Client) Disconnect(ctx context.Context) error {
	defer c.shutdown()

	if err := c.connectionManager.Close(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Client) Request(ctx context.Context, topic string, payload []byte) ([]byte, error) {
	rh, err := c.requestHandlerManager.AwaitRequestHandler(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Println("send request")
	resp, err := rh.Request(ctx, &paho.Publish{
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

		// context TODO
		connMgr, err := c.connectionManager.AwaitConnectionManager(context.TODO())
		if err != nil {
			// TODO err handle?
			fmt.Printf("await conn mgr: %+v\n", err)
		}

		// context TODO
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

func (c *Client) Subscribe(ctx context.Context, subscribe *paho.Subscribe) error {
	connMgr, err := c.connectionManager.AwaitConnectionManager(ctx)
	if err != nil {
		return err
	}
	if err := c.subscriptionManager.Subscribe(ctx, connMgr, subscribe); err != nil {
		return err
	}
	return nil
}

func awaitNewRequestHandler(ctx context.Context, connMgr *autopaho.ConnectionManager, config *paho.ClientConfig) (*rpc.Handler, error) {
	var delay time.Duration
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			rh, err := rpc.NewHandler(ctx, rpc.HandlerOpts{
				Conn:             connMgr,
				Router:           config.Router,
				ResponseTopicFmt: "%s/responses", // %s には ClientID がセットされる
				ClientID:         config.ClientID,
			})
			if err != nil {
				fmt.Printf("mqtt5: register request handler: %+v", err)
				delay = time.Second
				continue
			}
			return rh, nil
		}
	}
}
