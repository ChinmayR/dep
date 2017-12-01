package common

import (
	"context"
	"fmt"
	"strings"

	"code.uber.internal/engsec/wonka-go.git/internal/xhttp"
	"go.uber.org/zap"
)

var (
	clustoURL = "localhost:17949/dapi/trusteng/GetParentsForServer"
)

type pool struct {
	Name string `json:"name,omitempty"`
	Type string `json:"pool,omitempty"`
}

type clustoReply struct {
	Args  string `json:"Args,omitempty"`
	Data  []pool `json:"Data,omitempty"`
	Query string `json:"Query,omitempty"`
}

// GetPoolsForHost returns all of the clusto pools a given host is in.
func GetPoolsForHost(ctx context.Context, h string) (map[string]struct{}, error) {
	client := &xhttp.Client{}
	url := fmt.Sprintf("%s/%s", clustoURL, h)
	var reply clustoReply
	opts := &xhttp.CallOptions{CloseRequest: true}

	if err := xhttp.GetJSON(ctx, client, url, &reply, opts); err != nil {
		return nil, fmt.Errorf("error getting pools: %v", err)
	}

	// TODO(abg): Inject logger here
	zap.L().Info(
		"number of pools found for host",
		zap.Any("host", h),
		zap.Int("num_pools", len(reply.Data)))

	out := make(map[string]struct{}, len(reply.Data))
	for _, d := range reply.Data {
		out[strings.ToLower(d.Name)] = struct{}{}
	}

	return out, nil
}
