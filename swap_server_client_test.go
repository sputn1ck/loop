package loop

import (
	"context"
	"testing"

	"github.com/lightninglabs/loop/swapserverrpc"
	looptest "github.com/lightninglabs/loop/test"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type injectedSwapServerClient struct {
	swapserverrpc.SwapServerClient

	loopOutTermsCalls int
}

func (c *injectedSwapServerClient) LoopOutTerms(_ context.Context,
	_ *swapserverrpc.ServerLoopOutTermsRequest, _ ...grpc.CallOption) (
	*swapserverrpc.ServerLoopOutTerms, error) {

	c.loopOutTermsCalls++

	return &swapserverrpc.ServerLoopOutTerms{
		MinSwapAmount: 10,
		MaxSwapAmount: 20,
		MinCltvDelta:  30,
		MaxCltvDelta:  40,
	}, nil
}

// TestParseServerPubKey ensures that parseServerPubKey accepts a valid
// compressed public key and rejects keys with an invalid length or contents.
func TestParseServerPubKey(t *testing.T) {
	t.Parallel()

	_, pubKey := looptest.CreateKey(1)
	pubKeyBytes := pubKey.SerializeCompressed()

	parsedKey, err := parseServerPubKey("test key", pubKeyBytes)
	require.NoError(t, err)
	require.Equal(t, pubKeyBytes, parsedKey[:])

	_, err = parseServerPubKey("test key", pubKeyBytes[:32])
	require.ErrorContains(t, err, "invalid test key length")

	invalidKey := make([]byte, 33)
	_, err = parseServerPubKey("test key", invalidKey)
	require.ErrorContains(t, err, "invalid test key")
}

// TestInjectedSwapServerClient asserts that an alternate transport can supply
// the generated swap-server client without creating a native gRPC connection.
func TestInjectedSwapServerClient(t *testing.T) {
	t.Parallel()

	injectedClient := &injectedSwapServerClient{}
	client, err := newSwapServerClient(&ClientConfig{
		SwapServerClient: injectedClient,
	}, nil)
	require.NoError(t, err)
	require.Nil(t, client.conn)
	require.Same(t, injectedClient, client.server)

	terms, err := client.GetLoopOutTerms(t.Context(), "test")
	require.NoError(t, err)
	require.Equal(t, int64(10), int64(terms.MinSwapAmount))
	require.Equal(t, int64(20), int64(terms.MaxSwapAmount))
	require.Equal(t, int32(30), terms.MinCltvDelta)
	require.Equal(t, int32(40), terms.MaxCltvDelta)
	require.Equal(t, 1, injectedClient.loopOutTermsCalls)

	client.stop()
}
