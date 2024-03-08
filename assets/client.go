package assets

import (
	"bytes"
	"context"
	"os"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/lightninglabs/taproot-assets/tappsbt"
	"github.com/lightninglabs/taproot-assets/taprpc"
	wrpc "github.com/lightninglabs/taproot-assets/taprpc/assetwalletrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/mintrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/tapdevrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/universerpc"
	"github.com/lightninglabs/taproot-assets/tapsend"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/macaroons"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

var (
	maxMsgRecvSize = grpc.MaxCallRecvMsgSize(400 * 1024 * 1024)
)

type TapdConfig struct {
	Host         string `long:"host" description:"The host of the Tap daemon"`
	MacaroonPath string `long:"macaroonpath" description:"Path to the admin macaroon"`
	TLSPath      string `long:"tlspath" description:"Path to the TLS certificate"`
}

func DefaultTapdConfig() *TapdConfig {
	return &TapdConfig{
		Host:         "localhost:10031",
		MacaroonPath: "/home/kon-dev/docker/mounts/regtest/tapd-alice/admin.macaroon",
		TLSPath:      "/home/kon-dev/docker/mounts/regtest/tapd-alice/tls.cert",
	}

}

func NewTapdClient(config *TapdConfig) (*TapdClient, error) {
	// Create the client connection to the server.
	conn, err := getClientConn(config)
	if err != nil {
		return nil, err
	}

	// Create the TapdClient.
	client := &TapdClient{
		cc:                  conn,
		TaprootAssetsClient: taprpc.NewTaprootAssetsClient(conn),
		AssetWalletClient:   wrpc.NewAssetWalletClient(conn),
		MintClient:          mintrpc.NewMintClient(conn),
		UniverseClient:      universerpc.NewUniverseClient(conn),
		TapDevClient:        tapdevrpc.NewTapDevClient(conn),
	}

	return client, nil
}

func (c *TapdClient) Close() {
	c.cc.Close()
}

// TapdClient is a client for the Tap daemon.
type TapdClient struct {
	cc *grpc.ClientConn
	taprpc.TaprootAssetsClient
	wrpc.AssetWalletClient
	mintrpc.MintClient
	universerpc.UniverseClient
	tapdevrpc.TapDevClient
}

func (t *TapdClient) fundAndSignVpacket(ctx context.Context,
	vpkt *tappsbt.VPacket) (*tappsbt.VPacket, error) {

	// Fund the packet.
	var buf bytes.Buffer
	err := vpkt.Serialize(&buf)
	if err != nil {
		return nil, err
	}

	fundResp, err := t.FundVirtualPsbt(
		ctx, &wrpc.FundVirtualPsbtRequest{
			Template: &wrpc.FundVirtualPsbtRequest_Psbt{
				Psbt: buf.Bytes(),
			},
		},
	)
	if err != nil {
		return nil, err
	}

	// Sign the packet.
	signResp, err := t.SignVirtualPsbt(
		ctx, &wrpc.SignVirtualPsbtRequest{
			FundedPsbt: fundResp.FundedPsbt,
		},
	)
	if err != nil {
		return nil, err
	}

	return tappsbt.NewFromRawBytes(bytes.NewReader(signResp.SignedPsbt), false)
}

func (t *TapdClient) commitVirtualPsbts(ctx context.Context,
	vpkt *tappsbt.VPacket, feeRateSatPerKVByte chainfee.SatPerVByte) (
	*psbt.Packet, []*tappsbt.VPacket, []*tappsbt.VPacket,
	*wrpc.CommitVirtualPsbtsResponse, error) {

	htlcVPackets, err := tappsbt.Encode(vpkt)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	htlcBtcPkt, err := tapsend.PrepareAnchoringTemplate([]*tappsbt.VPacket{vpkt})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var buf bytes.Buffer
	err = htlcBtcPkt.Serialize(&buf)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	commitResponse, err := t.CommitVirtualPsbts(ctx, &wrpc.CommitVirtualPsbtsRequest{
		AnchorPsbt: buf.Bytes(),
		Fees: &wrpc.CommitVirtualPsbtsRequest_SatPerVbyte{
			SatPerVbyte: uint64(feeRateSatPerKVByte),
		},
		AnchorChangeOutput: &wrpc.CommitVirtualPsbtsRequest_Add{
			Add: true,
		},
		VirtualPsbts: [][]byte{
			htlcVPackets,
		},
	},
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	fundedPacket, err := psbt.NewFromRawBytes(
		bytes.NewReader(commitResponse.AnchorPsbt), false,
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	activePackets := make(
		[]*tappsbt.VPacket, len(commitResponse.VirtualPsbts),
	)
	for idx := range commitResponse.VirtualPsbts {
		activePackets[idx], err = tappsbt.Decode(
			commitResponse.VirtualPsbts[idx],
		)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	passivePackets := make(
		[]*tappsbt.VPacket, len(commitResponse.PassiveAssetPsbts),
	)
	for idx := range commitResponse.PassiveAssetPsbts {
		passivePackets[idx], err = tappsbt.Decode(
			commitResponse.PassiveAssetPsbts[idx],
		)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}

	return fundedPacket, activePackets, passivePackets, commitResponse, nil

}

func (t *TapdClient) logAndPublish(ctx context.Context, btcPkt *psbt.Packet,
	activeAssets []*tappsbt.VPacket, passiveAssets []*tappsbt.VPacket,
	commitResp *wrpc.CommitVirtualPsbtsResponse) (*taprpc.SendAssetResponse,
	error) {

	var buf bytes.Buffer
	err := btcPkt.Serialize(&buf)
	if err != nil {
		return nil, err
	}

	request := &wrpc.PublishAndLogRequest{
		AnchorPsbt:        buf.Bytes(),
		VirtualPsbts:      make([][]byte, len(activeAssets)),
		PassiveAssetPsbts: make([][]byte, len(passiveAssets)),
		ChangeOutputIndex: commitResp.ChangeOutputIndex,
		LndLockedUtxos:    commitResp.LndLockedUtxos,
	}

	for idx := range activeAssets {
		request.VirtualPsbts[idx], err = tappsbt.Encode(
			activeAssets[idx],
		)
		if err != nil {
			return nil, err
		}
	}
	for idx := range passiveAssets {
		request.PassiveAssetPsbts[idx], err = tappsbt.Encode(
			passiveAssets[idx],
		)
		if err != nil {
			return nil, err
		}
	}

	resp, err := t.PublishAndLogTransfer(ctx, request)
	if err != nil {
		return nil, err
	}

	return resp, nil
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
