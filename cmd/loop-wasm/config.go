package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	wasmsdk "github.com/lightninglabs/loop/sdk/wasm"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

const (
	defaultStartupTimeout = time.Minute
	defaultStopTimeout    = 30 * time.Second
)

// startRequest is the JSON configuration accepted by loopWasmCall("start").
type startRequest struct {
	DatabasePath  string `json:"database_path"`
	Seed          string `json:"seed"`
	Network       string `json:"network"`
	EsploraURL    string `json:"esplora_url"`
	LoopServerURL string `json:"loop_server_url"`

	L402Payer      string `json:"l402_payer"`
	L402MaxCostSat int64  `json:"l402_max_cost_sat"`

	PollIntervalMS      int64 `json:"poll_interval_ms"`
	MinRelayFeeSatPerKW int64 `json:"min_relay_fee_sat_per_kw"`
	StartupTimeoutMS    int64 `json:"startup_timeout_ms"`
}

// browserStartConfig contains the validated runtime settings and JS-only
// callback name needed by the syscall/js adapter.
type browserStartConfig struct {
	Runtime        wasmsdk.RuntimeConfig
	L402PayerName  string
	StartupTimeout time.Duration
}

type restoreRequest struct {
	DatabasePath string `json:"database_path"`
	Network      string `json:"network"`
	Bundle       string `json:"bundle"`
	FreshTarget  bool   `json:"fresh_target"`
	TimeoutMS    int64  `json:"timeout_ms"`
}

type browserRestoreConfig struct {
	Runtime wasmsdk.RestoreRecoveryConfig
	Timeout time.Duration
}

func parseStartConfig(body []byte) (browserStartConfig, error) {
	var request startRequest
	if len(body) == 0 {
		return browserStartConfig{}, errors.New(
			"browser Loop start configuration is required",
		)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return browserStartConfig{}, fmt.Errorf(
			"decode browser Loop start configuration: %w", err,
		)
	}

	seed, err := hex.DecodeString(strings.TrimSpace(request.Seed))
	if err != nil {
		return browserStartConfig{}, fmt.Errorf(
			"decode wallet seed as hex: %w", err,
		)
	}
	if len(seed) < 16 || len(seed) > 64 {
		return browserStartConfig{}, errors.New(
			"wallet seed must contain between 16 and 64 bytes",
		)
	}

	chainParams, err := networkParams(request.Network)
	if err != nil {
		return browserStartConfig{}, err
	}
	if request.PollIntervalMS < 0 {
		return browserStartConfig{}, errors.New(
			"poll interval must not be negative",
		)
	}
	if request.MinRelayFeeSatPerKW < 0 {
		return browserStartConfig{}, errors.New(
			"minimum relay fee must not be negative",
		)
	}
	if request.L402MaxCostSat < 0 {
		return browserStartConfig{}, errors.New(
			"maximum L402 cost must not be negative",
		)
	}
	if request.StartupTimeoutMS < 0 {
		return browserStartConfig{}, errors.New(
			"startup timeout must not be negative",
		)
	}

	startupTimeout := defaultStartupTimeout
	if request.StartupTimeoutMS > 0 {
		startupTimeout = time.Duration(request.StartupTimeoutMS) *
			time.Millisecond
	}

	return browserStartConfig{
		Runtime: wasmsdk.RuntimeConfig{
			DatabasePath: strings.TrimSpace(request.DatabasePath),
			Seed:         seed,
			ChainParams:  chainParams,
			EsploraURL:   strings.TrimSpace(request.EsploraURL),
			SwapServerURL: strings.TrimSpace(
				request.LoopServerURL,
			),
			PollInterval: time.Duration(request.PollIntervalMS) *
				time.Millisecond,
			MinRelayFee: chainfee.SatPerKWeight(
				request.MinRelayFeeSatPerKW,
			),
			L402MaxCost: btcutil.Amount(request.L402MaxCostSat),
		},
		L402PayerName:  strings.TrimSpace(request.L402Payer),
		StartupTimeout: startupTimeout,
	}, nil
}

func parseRestoreConfig(body []byte) (browserRestoreConfig, error) {
	var request restoreRequest
	if len(body) == 0 {
		return browserRestoreConfig{}, errors.New(
			"browser Loop restore configuration is required",
		)
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return browserRestoreConfig{}, fmt.Errorf(
			"decode browser Loop restore configuration: %w", err,
		)
	}
	chainParams, err := networkParams(request.Network)
	if err != nil {
		return browserRestoreConfig{}, err
	}
	if strings.TrimSpace(request.DatabasePath) == "" {
		return browserRestoreConfig{}, errors.New(
			"restore database path is required",
		)
	}
	if strings.TrimSpace(request.Bundle) == "" {
		return browserRestoreConfig{}, errors.New(
			"recovery bundle is required",
		)
	}
	if !request.FreshTarget {
		return browserRestoreConfig{}, errors.New(
			"restore requires fresh_target to be true",
		)
	}
	if request.TimeoutMS < 0 {
		return browserRestoreConfig{}, errors.New(
			"restore timeout must not be negative",
		)
	}

	timeout := defaultStartupTimeout
	if request.TimeoutMS > 0 {
		timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}

	return browserRestoreConfig{
		Runtime: wasmsdk.RestoreRecoveryConfig{
			DatabasePath: strings.TrimSpace(request.DatabasePath),
			ChainParams:  chainParams,
			Bundle:       []byte(request.Bundle),
			FreshTarget:  true,
		},
		Timeout: timeout,
	}, nil
}

func networkParams(network string) (*chaincfg.Params, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "mainnet":
		return &chaincfg.MainNetParams, nil

	case "testnet", "testnet3":
		return &chaincfg.TestNet3Params, nil

	case "testnet4":
		return &chaincfg.TestNet4Params, nil

	case "signet":
		return &chaincfg.SigNetParams, nil

	case "regtest":
		return &chaincfg.RegressionNetParams, nil

	default:
		return nil, fmt.Errorf("unsupported Bitcoin network %q", network)
	}
}
