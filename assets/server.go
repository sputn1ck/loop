package assets

import (
	"context"
	"errors"

	clientrpc "github.com/lightninglabs/loop/looprpc"
)

type AssetsClientServer struct {
	manager *AssetsSwapManager

	clientrpc.UnimplementedAssetsClientServer
}

func NewAssetsServer(manager *AssetsSwapManager) *AssetsClientServer {
	return &AssetsClientServer{}
}

func (a *AssetsClientServer) SwapOut(_ context.Context, _ *clientrpc.SwapOutRequest) (
	*clientrpc.SwapOutResponse, error) {

	return nil, errors.New("foobar")
}
