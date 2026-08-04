package wasm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testGenesisHash = "000000000019d6689c085ae165831e93" +
	"4ff763ae46a2a6c172b3f1b60a8ce26f"

func TestRecoveryBundleRoundTrip(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	bundle, err := BuildRecoveryBundle(
		context.Background(), RecoveryBundleConfig{
			Network: RecoveryNetwork{
				Name:        "mainnet",
				GenesisHash: testGenesisHash,
			},
			CreatedAt: createdAt,
			Databases: []DatabaseSource{
				{
					Name:          "loop.db",
					SchemaVersion: 7,
					Encoding:      SQLiteDatabaseEncoding,
					Source: staticDumpSource(
						[]byte("sqlite database image"),
					),
				},
				{
					Name:          "wallet.sql",
					SchemaVersion: 2,
					Encoding:      SQLiteSQLDumpEncoding,
					Source: staticDumpSource(
						[]byte("BEGIN; COMMIT;"),
					),
				},
			},
			Artifacts: []ArtifactSource{
				{
					Name:          WalletSeedArtifactName,
					SchemaVersion: 1,
					Encoding:      OpaqueArtifactEncoding,
					Source: staticDumpSource(
						[]byte("wallet seed/key material"),
					),
				},
				{
					Name:          L402AuthorizationArtifactName,
					SchemaVersion: 1,
					Encoding:      UTF8ArtifactEncoding,
					Source: staticDumpSource(
						[]byte("L402 persistent-authorization"),
					),
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	if bundle.FormatVersion != RecoveryBundleVersion {
		t.Fatalf("unexpected version: %v", bundle.FormatVersion)
	}
	if len(bundle.ChecksumSHA256) != 64 {
		t.Fatalf("unexpected checksum: %v", bundle.ChecksumSHA256)
	}
	if len(bundle.Databases) != 2 {
		t.Fatalf("unexpected database count: %v", len(bundle.Databases))
	}
	if len(bundle.Artifacts) != 2 {
		t.Fatalf("unexpected artifact count: %v", len(bundle.Artifacts))
	}
	if bundle.Artifacts[0].Name != WalletSeedArtifactName ||
		bundle.Artifacts[1].Name != L402AuthorizationArtifactName {

		t.Fatalf("unexpected recovery artifacts: %v", bundle.Artifacts)
	}

	encoded, err := MarshalRecoveryBundle(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	parsed, err := ParseRecoveryBundle(encoded)
	if err != nil {
		t.Fatalf("parse bundle: %v", err)
	}
	if !reflect.DeepEqual(parsed, bundle) {
		t.Fatalf("round trip mismatch:\nwant: %+v\ngot:  %+v", bundle, parsed)
	}
}

func TestRecoveryBundleRejectsTampering(t *testing.T) {
	t.Parallel()

	bundle := testRecoveryBundle(t)
	bundle.Databases[0].Data[0] ^= 1
	if err := bundle.Validate(); err == nil ||
		!strings.Contains(err.Error(), "database \"loop.db\" checksum mismatch") {

		t.Fatalf("unexpected validation error: %v", err)
	}

	bundle = testRecoveryBundle(t)
	bundle.Artifacts[0].Data[0] ^= 1
	if err := bundle.Validate(); err == nil || !strings.Contains(
		err.Error(), "recovery artifact \"wallet_seed\" checksum mismatch",
	) {

		t.Fatalf("unexpected validation error: %v", err)
	}

	bundle = testRecoveryBundle(t)
	bundle.Network.Name = "testnet3"
	if err := bundle.Validate(); err == nil ||
		!strings.Contains(err.Error(), "recovery bundle checksum mismatch") {

		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRecoveryBundleSourceError(t *testing.T) {
	t.Parallel()

	sourceError := errors.New("snapshot unavailable")
	_, err := BuildRecoveryBundle(
		context.Background(), RecoveryBundleConfig{
			Network: testRecoveryNetwork(),
			Databases: []DatabaseSource{{
				Name:          "loop.db",
				SchemaVersion: 1,
				Encoding:      SQLiteDatabaseEncoding,
				Source: DumpSourceFunc(
					func(context.Context) ([]byte, error) {
						return nil, sourceError
					},
				),
			}},
		},
	)
	if !errors.Is(err, sourceError) {
		t.Fatalf("source error not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), `dump database "loop.db"`) {
		t.Fatalf("missing database context: %v", err)
	}
}

func TestRecoveryBundleCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BuildRecoveryBundle(ctx, RecoveryBundleConfig{
		Network: testRecoveryNetwork(),
		Databases: []DatabaseSource{{
			Name:          "loop.db",
			SchemaVersion: 1,
			Encoding:      SQLiteDatabaseEncoding,
			Source:        staticDumpSource([]byte("database")),
		}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecoveryBundleRejectsDuplicateNames(t *testing.T) {
	t.Parallel()

	_, err := BuildRecoveryBundle(
		context.Background(), RecoveryBundleConfig{
			Network: testRecoveryNetwork(),
			Databases: []DatabaseSource{
				{
					Name:          "loop.db",
					SchemaVersion: 1,
					Encoding:      SQLiteDatabaseEncoding,
					Source: staticDumpSource(
						[]byte("database one"),
					),
				},
				{
					Name:          "LOOP.DB",
					SchemaVersion: 1,
					Encoding:      SQLiteDatabaseEncoding,
					Source: staticDumpSource(
						[]byte("database two"),
					),
				},
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate database name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRecoveryBundleStrictJSON(t *testing.T) {
	t.Parallel()

	bundle := testRecoveryBundle(t)
	encoded, err := MarshalRecoveryBundle(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}

	withUnknownField := append(
		append([]byte(nil), encoded[:len(encoded)-1]...),
		[]byte(`,"unexpected":true}`)...,
	)
	_, err = ParseRecoveryBundle(withUnknownField)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected unknown-field error: %v", err)
	}

	_, err = ParseRecoveryBundle(append(encoded, []byte(` {}`)...))
	if err == nil || !strings.Contains(err.Error(), "trailing JSON value") {
		t.Fatalf("unexpected trailing-value error: %v", err)
	}
}

func TestRecoveryBundleRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		modify func(*RecoveryBundleConfig)
	}{
		{
			name: "invalid genesis hash",
			modify: func(config *RecoveryBundleConfig) {
				config.Network.GenesisHash = "not-a-hash"
			},
		},
		{
			name: "missing schema version",
			modify: func(config *RecoveryBundleConfig) {
				config.Databases[0].SchemaVersion = 0
			},
		},
		{
			name: "missing encoding",
			modify: func(config *RecoveryBundleConfig) {
				config.Databases[0].Encoding = ""
			},
		},
		{
			name: "missing source",
			modify: func(config *RecoveryBundleConfig) {
				config.Databases[0].Source = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := RecoveryBundleConfig{
				Network: testRecoveryNetwork(),
				Databases: []DatabaseSource{{
					Name:          "loop.db",
					SchemaVersion: 1,
					Encoding:      SQLiteDatabaseEncoding,
					Source: staticDumpSource(
						[]byte("database"),
					),
				}},
			}
			test.modify(&config)

			if _, err := BuildRecoveryBundle(
				context.Background(), config,
			); err == nil {

				t.Fatalf("invalid metadata accepted")
			}
		})
	}
}

func testRecoveryBundle(t *testing.T) *RecoveryBundle {
	t.Helper()

	bundle, err := BuildRecoveryBundle(
		context.Background(), RecoveryBundleConfig{
			Network:   testRecoveryNetwork(),
			CreatedAt: time.Unix(1_775_301_600, 0).UTC(),
			Databases: []DatabaseSource{{
				Name:          "loop.db",
				SchemaVersion: 1,
				Encoding:      SQLiteDatabaseEncoding,
				Source:        staticDumpSource([]byte("database")),
			}},
			Artifacts: []ArtifactSource{
				{
					Name:          WalletSeedArtifactName,
					SchemaVersion: 1,
					Encoding:      OpaqueArtifactEncoding,
					Source: staticDumpSource(
						[]byte("wallet seed/key material"),
					),
				},
				{
					Name:          L402AuthorizationArtifactName,
					SchemaVersion: 1,
					Encoding:      UTF8ArtifactEncoding,
					Source: staticDumpSource(
						[]byte("L402 persistent-authorization"),
					),
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("build test bundle: %v", err)
	}

	return bundle
}

func testRecoveryNetwork() RecoveryNetwork {
	return RecoveryNetwork{
		Name:        "mainnet",
		GenesisHash: testGenesisHash,
	}
}

func staticDumpSource(data []byte) DumpSource {
	return DumpSourceFunc(
		func(context.Context) ([]byte, error) {
			return append([]byte(nil), data...), nil
		},
	)
}
