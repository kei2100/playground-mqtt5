package mqtt5

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

var brokerURL, _ = url.Parse("mqtt://127.0.0.1:1883")

func init() {
	if p := os.Getenv("HOST_MQTT_PORT"); p != "" {
		brokerURL, _ = url.Parse(fmt.Sprintf("mqtt://127.0.0.1:%s", p))
	}
}

func TestClient_Connect(t *testing.T) {
	ctx := signalContext(t, context.Background())
	cli := NewClient(brokerURL)
	if err := cli.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cli.Disconnect(ctx); err != nil {
		t.Error(err)
	}
}

func TestClient_Request(t *testing.T) {
	ctx := signalContext(t, context.Background())

	cli1 := NewClient(brokerURL)
	if err := cli1.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli1.Disconnect(ctx)
	})

	cli2 := NewClient(brokerURL)
	if err := cli2.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli2.Disconnect(ctx)
	})
	cli2.RegisterRequestHandler("foo", func(req []byte) (resp []byte) {
		return req
	})
	if err := cli2.Subscribe(ctx, "foo"); err != nil {
		t.Fatal(err)
	}

	resp, err := cli1.Request(ctx, "foo", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(resp))
}

func signalContext(t *testing.T, ctx context.Context) context.Context {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	t.Cleanup(cancel)
	return ctx
}
