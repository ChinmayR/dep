// Code generated by thriftrw-plugin-yarpc
// @generated

package weatherclient

import (
	"context"
	tchannel "github.com/uber/tchannel-go"
	"go.uber.org/thriftrw/wire"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/encoding/thrift"
	"go.uber.org/yarpc/encoding/thrift/thriftrw-plugin-yarpc/internal/tests/weather"
	"reflect"
)

// Interface is a client for the Weather service.
type Interface interface {
	Check(
		ctx context.Context,
		opts ...yarpc.CallOption,
	) (string, error)
}

// New builds a new client for the Weather service.
//
// 	client := weatherclient.New(dispatcher.ClientConfig("weather"))
func New(c transport.ClientConfig, opts ...thrift.ClientOption) Interface {
	return client{
		c: thrift.New(thrift.Config{
			Service:      "Weather",
			ClientConfig: c,
		}, opts...),
	}
}

func init() {
	yarpc.RegisterClientBuilder(
		func(c transport.ClientConfig, f reflect.StructField) Interface {
			return New(c, thrift.ClientBuilderOptions(c, f)...)
		},
	)
}

type client struct {
	c thrift.Client
}

func (c client) Check(
	ctx context.Context,
	opts ...yarpc.CallOption,
) (success string, err error) {

	args := weather.Weather_Check_Helper.Args()

	ctx = tchannel.WithoutHeaders(ctx)
	var body wire.Value
	body, err = c.c.Call(ctx, args, opts...)
	if err != nil {
		return
	}

	var result weather.Weather_Check_Result
	if err = result.FromWire(body); err != nil {
		return
	}

	success, err = weather.Weather_Check_Helper.UnwrapResponse(&result)
	return
}
