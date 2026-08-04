package main

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
)

func TestParseStartConfig(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	config, err := parseStartConfig([]byte(`{
		"database_path":"loop-browser.db",
		"seed":"` + hex.EncodeToString(seed) + `",
		"network":"testnet4",
		"esplora_url":"https://esplora.example",
		"loop_server_url":"https://loop.example",
		"l402_payer":"loopWasmPayInvoice",
		"l402_max_cost_sat":1000,
		"poll_interval_ms":2500,
		"min_relay_fee_sat_per_kw":253,
		"startup_timeout_ms":5000
	}`))
	require.NoError(t, err)
	require.Equal(t, "loop-browser.db", config.Runtime.DatabasePath)
	require.Equal(t, seed, config.Runtime.Seed)
	require.Equal(t, &chaincfg.TestNet4Params, config.Runtime.ChainParams)
	require.Equal(t, "https://esplora.example", config.Runtime.EsploraURL)
	require.Equal(t, "https://loop.example", config.Runtime.SwapServerURL)
	require.Equal(t, "loopWasmPayInvoice", config.L402PayerName)
	require.Equal(t, 2500*time.Millisecond, config.Runtime.PollInterval)
	require.Equal(t, 5*time.Second, config.StartupTimeout)
}

func TestParseStartConfigRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	valid := `{
		"database_path":"loop-browser.db",
		"seed":"00000000000000000000000000000000",
		"network":"regtest",
		"esplora_url":"https://esplora.example",
		"loop_server_url":"https://loop.example"
	}`

	tests := []struct {
		name     string
		request  string
		expected string
	}{
		{
			name:     "missing config",
			request:  "",
			expected: "configuration is required",
		},
		{
			name: "seed",
			request: `{
				"seed":"00",
				"network":"regtest"
			}`,
			expected: "between 16 and 64 bytes",
		},
		{
			name: "network",
			request: `{
				"seed":"00000000000000000000000000000000",
				"network":"unknown"
			}`,
			expected: "unsupported Bitcoin network",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseStartConfig([]byte(test.request))
			require.ErrorContains(t, err, test.expected)
		})
	}

	config, err := parseStartConfig([]byte(valid))
	require.NoError(t, err)
	require.Equal(t, defaultStartupTimeout, config.StartupTimeout)
}

func TestParseRestoreConfig(t *testing.T) {
	t.Parallel()

	config, err := parseRestoreConfig([]byte(`{
		"database_path":"restored.db",
		"network":"signet",
		"bundle":"{\"format_version\":1}",
		"fresh_target":true,
		"timeout_ms":9000
	}`))
	require.NoError(t, err)
	require.Equal(t, "restored.db", config.Runtime.DatabasePath)
	require.Equal(t, &chaincfg.SigNetParams, config.Runtime.ChainParams)
	require.Equal(t, []byte(`{"format_version":1}`), config.Runtime.Bundle)
	require.True(t, config.Runtime.FreshTarget)
	require.Equal(t, 9*time.Second, config.Timeout)

	_, err = parseRestoreConfig([]byte(`{
		"database_path":"restored.db",
		"network":"signet",
		"bundle":"{}"
	}`))
	require.ErrorContains(t, err, "fresh_target")
}
