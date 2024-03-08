package assets

import (
	"context"
	"sync"

	"github.com/lightninglabs/lndclient"
	loop_rpc "github.com/lightninglabs/loop/swapserverrpc"
)

const (
	// DefaultSwapExpiry is the default expiry for a swap in blocks.
	DefaultSwapExpiry = 24

	FixedPrepayCost = 30000

	ServerKeyFamily = 696969
)

type SwapStore interface {
	// CreateAssetSwapOut creates a new asset swap out in the store.
	CreateAssetSwapOut(ctx context.Context, swap *SwapOut) error

	// GetAssetSwapOut gets an asset swap out from the store.
	GetAssetSwapOut(ctx context.Context, swapHash []byte) (*SwapOut, error)

	// UpdateAssetSwapOut updates an asset swap out in the store.
	UpdateAssetSwapOut(ctx context.Context, swap *SwapOut) error
}

type Config struct {
	AssetClient      *TapdClient
	Wallet           lndclient.WalletKitClient
	Signer           lndclient.SignerClient
	ChainNotifier    lndclient.ChainNotifierClient
	Router           lndclient.RouterClient
	LndClient        lndclient.LightningClient
	Store            SwapStore
	InstantOutClient loop_rpc.AssetsSwapServerClient
}

type AssetsSwapManager struct {
	cfg *Config

	blockHeight int32
	runCtx      context.Context

	sync.Mutex
}

func NewAssetSwapServer(config *Config) *AssetsSwapManager {
	return &AssetsSwapManager{
		cfg: config,
	}
}

func (m *AssetsSwapManager) Run(ctx context.Context, height int32) error {
	m.runCtx = ctx
	m.blockHeight = height

	for {
		select {
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
