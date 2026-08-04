package browserwallet

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/stretchr/testify/require"
)

// TestFeeEstimate verifies that a missing exact target uses the most
// conservative published bucket that does not exceed the request.
func TestFeeEstimate(t *testing.T) {
	t.Parallel()

	estimates := map[string]float64{
		"1": 10,
		"3": 5,
		"6": 2,
	}

	rate, err := feeEstimate(estimates, 4)
	require.NoError(t, err)
	require.Equal(t, float64(5), rate)

	rate, err = feeEstimate(estimates, 1)
	require.NoError(t, err)
	require.Equal(t, float64(10), rate)

	_, err = feeEstimate(nil, 6)
	require.ErrorIs(t, err, errNoFeeEstimates)
}

// TestRegtestEmptyFeeEstimateFallback verifies that an empty Esplora fee map
// uses the configured relay floor only when the regtest fallback is enabled.
func TestRegtestEmptyFeeEstimateFallback(t *testing.T) {
	t.Parallel()

	esplora := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		_ *http.Request) {

		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(esplora.Close)

	client, err := NewEsploraClient(esplora.URL, esplora.Client())
	require.NoError(t, err)

	const relayFloor = chainfee.SatPerKWeight(250)
	wallet := &WalletKit{
		esplora:               client,
		minRelayFee:           relayFloor,
		allowEmptyFeeEstimate: true,
	}

	rate, err := wallet.EstimateFeeRate(t.Context(), 6)
	require.NoError(t, err)
	require.Equal(t, relayFloor, rate)

	wallet.allowEmptyFeeEstimate = false
	_, err = wallet.EstimateFeeRate(t.Context(), 6)
	require.ErrorIs(t, err, errNoFeeEstimates)
}
