package assets

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightninglabs/taproot-assets/rfqmath"
	"github.com/lightninglabs/taproot-assets/taprpc"
	"github.com/lightninglabs/taproot-assets/taprpc/priceoraclerpc"
	"github.com/lightninglabs/taproot-assets/taprpc/rfqrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/tapchannelrpc"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

var (
	maxMsgRecvSize = grpc.MaxCallRecvMsgSize(400 * 1024 * 1024)
)

// TapdConfig is a struct that holds the configuration options to connect to a
// taproot assets daemon.
type TapdConfig struct {
	Host         string `long:"host" description:"The host of the Tap daemon"`
	MacaroonPath string `long:"macaroonpath" description:"Path to the admin macaroon"`
	TLSPath      string `long:"tlspath" description:"Path to the TLS certificate"`
}

// DefaultTapdConfig returns a default configuration to connect to a taproot
// assets daemon.
func DefaultTapdConfig() *TapdConfig {
	return &TapdConfig{
		Host:         "",
		MacaroonPath: "",
		TLSPath:      "",
	}
}

// TapdClient is a client for the Tap daemon.
type TapdClient struct {
	cc *grpc.ClientConn
	taprpc.TaprootAssetsClient
	tapchannelrpc.TaprootAssetChannelsClient
	priceoraclerpc.PriceOracleClient
	rfqrpc.RfqClient
}

// NewTapdClient retusn a new taproot assets client.
func NewTapdClient(config *TapdConfig) (*TapdClient, error) {
	// Create the client connection to the server.
	conn, err := getClientConn(config)
	if err != nil {
		return nil, err
	}

	// Create the TapdClient.
	client := &TapdClient{
		cc:                         conn,
		TaprootAssetsClient:        taprpc.NewTaprootAssetsClient(conn),
		TaprootAssetChannelsClient: tapchannelrpc.NewTaprootAssetChannelsClient(conn),
		PriceOracleClient:          priceoraclerpc.NewPriceOracleClient(conn),
		RfqClient:                  rfqrpc.NewRfqClient(conn),
	}

	return client, nil
}

// Close closes the client connection to the server.
func (c *TapdClient) Close() {
	c.cc.Close()
}

// GetSatAmtFromRfq returns the amount in satoshis for the given asset amount
// and pay rate.
func GetSatAmtFromRfq(assetAmt btcutil.Amount,
	payRate *rfqrpc.FixedPoint) (btcutil.Amount, error) {

	coefficient := new(big.Int)
	coefficient, ok := coefficient.SetString(payRate.Coefficient, 10)
	if !ok {
		return 0, fmt.Errorf("failed to parse coefficient %v",
			payRate.Coefficient)
	}

	amt := rfqmath.FixedPointFromUint64[rfqmath.BigInt](
		uint64(assetAmt), 0,
	)

	price := rfqmath.FixedPoint[rfqmath.BigInt]{
		Coefficient: rfqmath.NewBigInt(coefficient),
		Scale:       uint8(payRate.Scale),
	}

	msats := rfqmath.UnitsToMilliSatoshi(amt, price)
	return msats.ToSatoshis(), nil
}

// GetRfqForAsset returns a RFQ for the given asset with the given amount and
// to the given peer.
func (c *TapdClient) GetRfqForAsset(ctx context.Context,
	amt btcutil.Amount, assetId, peerPubkey []byte) (
	*rfqrpc.PeerAcceptedSellQuote, error) {

	// TODO(sputn1ck): magic value, should be configurable?
	feeLimit, err := lnrpc.UnmarshallAmt(
		int64(amt)+int64(amt.MulF64(1.1)), 0,
	)
	if err != nil {
		return nil, err
	}

	// TODO(sputn1ck): magic value, should be configurable?
	expiry := time.Now().Add(1 * time.Hour).Unix()

	rfq, err := c.RfqClient.AddAssetSellOrder(
		ctx, &rfqrpc.AddAssetSellOrderRequest{
			AssetSpecifier: &rfqrpc.AssetSpecifier{
				Id: &rfqrpc.AssetSpecifier_AssetId{
					AssetId: assetId,
				},
			},
			PeerPubKey:     peerPubkey,
			PaymentMaxAmt:  uint64(feeLimit),
			Expiry:         uint64(expiry),
			TimeoutSeconds: 60,
		})
	if err != nil {
		return nil, err
	}
	if rfq.GetInvalidQuote() != nil {
		return nil, fmt.Errorf("invalid RFQ: %v", rfq.GetInvalidQuote())
	}
	if rfq.GetRejectedQuote() != nil {
		return nil, fmt.Errorf("rejected RFQ: %v",
			rfq.GetRejectedQuote())
	}

	if rfq.GetAcceptedQuote() != nil {
		return rfq.GetAcceptedQuote(), nil
	}

	return nil, fmt.Errorf("no accepted quote")
}

func getClientConn(config *TapdConfig) (*grpc.ClientConn, error) {
	// Load the specified TLS certificate and build transport credentials.
	creds, err := credentials.NewClientTLSFromFile(config.TLSPath, "")
	if err != nil {
		return nil, err
	}

	// Load the specified macaroon file.
	macBytes, err := os.ReadFile(config.MacaroonPath)
	if err != nil {
		return nil, err
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, err
	}

	macaroon, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, err
	}
	// Create the DialOptions with the macaroon credentials.
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(macaroon),
		grpc.WithDefaultCallOptions(maxMsgRecvSize),
	}

	// Dial the gRPC server.
	conn, err := grpc.Dial(config.Host, opts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
