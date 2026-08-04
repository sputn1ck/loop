package wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// RecoveryBundleVersion is the recovery bundle schema version written by
	// this package.
	RecoveryBundleVersion uint32 = 1

	// SQLiteDatabaseEncoding identifies a raw SQLite database image.
	SQLiteDatabaseEncoding = "application/vnd.sqlite3"

	// SQLiteSQLDumpEncoding identifies a textual SQLite SQL dump.
	SQLiteSQLDumpEncoding = "application/sql;dialect=sqlite3"

	// WalletSeedArtifactName is the conventional wallet seed/key artifact
	// name.
	WalletSeedArtifactName = "wallet_seed"

	// L402AuthorizationArtifactName is the conventional persistent L402
	// authorization artifact name.
	L402AuthorizationArtifactName = "l402_authorization"

	// OpaqueArtifactEncoding identifies opaque binary recovery material.
	OpaqueArtifactEncoding = "application/octet-stream"

	// UTF8ArtifactEncoding identifies UTF-8 recovery material.
	UTF8ArtifactEncoding = "text/plain;charset=utf-8"
)

// RecoveryNetwork identifies the chain for which a recovery bundle is valid.
// Name is descriptive; GenesisHash is the authoritative identity.
type RecoveryNetwork struct {
	Name        string `json:"name"`
	GenesisHash string `json:"genesis_hash"`
}

// DatabaseDump is a checksummed database image and its restore metadata.
type DatabaseDump struct {
	Name           string `json:"name"`
	SchemaVersion  uint32 `json:"schema_version"`
	Encoding       string `json:"encoding"`
	Data           []byte `json:"data"`
	ChecksumSHA256 string `json:"checksum_sha256"`
}

// RecoveryArtifact is checksummed non-database state required for recovery.
// Typical artifacts include wallet seed/key material and persistent L402
// authorization credentials.
type RecoveryArtifact struct {
	Name           string `json:"name"`
	SchemaVersion  uint32 `json:"schema_version"`
	Encoding       string `json:"encoding"`
	Data           []byte `json:"data"`
	ChecksumSHA256 string `json:"checksum_sha256"`
}

// RecoveryBundle contains all state needed to recover the browser client.
// Checksums detect accidental corruption only; they do not provide encryption
// or authentication. Because artifacts can contain wallet seed/key material
// and reusable L402 credentials, callers MUST encrypt the serialized bundle
// before it leaves trusted browser storage.
type RecoveryBundle struct {
	FormatVersion  uint32             `json:"format_version"`
	Network        RecoveryNetwork    `json:"network"`
	CreatedAt      time.Time          `json:"created_at"`
	Databases      []DatabaseDump     `json:"databases"`
	Artifacts      []RecoveryArtifact `json:"artifacts,omitempty"`
	ChecksumSHA256 string             `json:"checksum_sha256"`
}

// DumpSource returns a consistent recovery database or artifact image.
type DumpSource interface {
	Dump(context.Context) ([]byte, error)
}

// DumpSourceFunc adapts a function into a DumpSource.
type DumpSourceFunc func(context.Context) ([]byte, error)

// Dump calls the wrapped recovery dump function.
func (f DumpSourceFunc) Dump(ctx context.Context) ([]byte, error) {
	if f == nil {
		return nil, errors.New("recovery dump function is nil")
	}

	return f(ctx)
}

// DatabaseSource describes one persistent database and how to dump it.
type DatabaseSource struct {
	Name          string
	SchemaVersion uint32
	Encoding      string
	Source        DumpSource
}

// ArtifactSource describes non-database state and how to snapshot it. Wallet
// seeds and persistent L402 authorization are modeled here as first-class
// recovery inputs.
type ArtifactSource struct {
	Name          string
	SchemaVersion uint32
	Encoding      string
	Source        DumpSource
}

// RecoveryBundleConfig defines the inputs for a recovery snapshot. If
// CreatedAt is zero, BuildRecoveryBundle uses the current time.
type RecoveryBundleConfig struct {
	Network   RecoveryNetwork
	CreatedAt time.Time
	Databases []DatabaseSource
	Artifacts []ArtifactSource
}

// BuildRecoveryBundle snapshots each configured recovery source and calculates
// its integrity metadata. Callers must stop writers or otherwise coordinate
// the sources if consistency across several entries is required.
func BuildRecoveryBundle(ctx context.Context,
	config RecoveryBundleConfig) (*RecoveryBundle, error) {

	if err := validateSourceConfig(config); err != nil {
		return nil, err
	}

	createdAt := config.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	bundle := &RecoveryBundle{
		FormatVersion: RecoveryBundleVersion,
		Network: RecoveryNetwork{
			Name:        strings.TrimSpace(config.Network.Name),
			GenesisHash: strings.ToLower(config.Network.GenesisHash),
		},
		CreatedAt: createdAt.UTC().Round(0),
		Databases: make([]DatabaseDump, 0, len(config.Databases)),
		Artifacts: make(
			[]RecoveryArtifact, 0, len(config.Artifacts),
		),
	}

	for _, database := range config.Databases {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("create recovery bundle: %w", err)
		}

		data, err := database.Source.Dump(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"dump database %q: %w", database.Name, err,
			)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf(
				"dump database %q: empty database image",
				database.Name,
			)
		}

		bundle.Databases = append(bundle.Databases, DatabaseDump{
			Name:           strings.TrimSpace(database.Name),
			SchemaVersion:  database.SchemaVersion,
			Encoding:       strings.TrimSpace(database.Encoding),
			Data:           append([]byte(nil), data...),
			ChecksumSHA256: checksum(data),
		})
	}

	for _, artifact := range config.Artifacts {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("create recovery bundle: %w", err)
		}

		data, err := artifact.Source.Dump(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"dump recovery artifact %q: %w", artifact.Name, err,
			)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf(
				"dump recovery artifact %q: empty artifact image",
				artifact.Name,
			)
		}

		bundle.Artifacts = append(bundle.Artifacts, RecoveryArtifact{
			Name:           strings.TrimSpace(artifact.Name),
			SchemaVersion:  artifact.SchemaVersion,
			Encoding:       strings.TrimSpace(artifact.Encoding),
			Data:           append([]byte(nil), data...),
			ChecksumSHA256: checksum(data),
		})
	}

	bundleChecksum, err := checksumRecoveryBundle(bundle)
	if err != nil {
		return nil, err
	}
	bundle.ChecksumSHA256 = bundleChecksum

	return bundle, nil
}

// MarshalRecoveryBundle validates and serializes a recovery bundle as JSON.
// Database and artifact images are encoded as base64 by encoding/json. The
// returned bytes contain plaintext secrets unless the caller encrypts them.
func MarshalRecoveryBundle(bundle *RecoveryBundle) ([]byte, error) {
	if bundle == nil {
		return nil, errors.New("recovery bundle is nil")
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal recovery bundle: %w", err)
	}

	return encoded, nil
}

// ParseRecoveryBundle strictly parses and validates a recovery bundle.
func ParseRecoveryBundle(encoded []byte) (*RecoveryBundle, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	bundle := new(RecoveryBundle)
	if err := decoder.Decode(bundle); err != nil {
		return nil, fmt.Errorf("decode recovery bundle: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}

	return bundle, nil
}

// Validate verifies recovery metadata and all checksums without mutating the
// bundle.
func (b *RecoveryBundle) Validate() error {
	if b == nil {
		return errors.New("recovery bundle is nil")
	}
	if b.FormatVersion != RecoveryBundleVersion {
		return fmt.Errorf(
			"unsupported recovery bundle version %d", b.FormatVersion,
		)
	}
	if err := validateNetwork(b.Network); err != nil {
		return err
	}
	if b.CreatedAt.IsZero() {
		return errors.New("recovery bundle creation time is required")
	}
	_, offset := b.CreatedAt.Zone()
	if offset != 0 {
		return errors.New("recovery bundle creation time must use UTC")
	}
	if len(b.Databases) == 0 {
		return errors.New("recovery bundle contains no databases")
	}

	recoveryNames := make(
		map[string]struct{}, len(b.Databases)+len(b.Artifacts),
	)
	for i := range b.Databases {
		database := &b.Databases[i]
		if err := validateDatabaseMetadata(
			database.Name, database.SchemaVersion, database.Encoding,
		); err != nil {

			return fmt.Errorf("database %d: %w", i, err)
		}

		nameKey := strings.ToLower(database.Name)
		if _, ok := recoveryNames[nameKey]; ok {
			return fmt.Errorf("duplicate database name %q", database.Name)
		}
		recoveryNames[nameKey] = struct{}{}

		if len(database.Data) == 0 {
			return fmt.Errorf("database %q has an empty image", database.Name)
		}
		if err := validateChecksum(database.ChecksumSHA256); err != nil {
			return fmt.Errorf(
				"database %q checksum: %w", database.Name, err,
			)
		}
		if checksum(database.Data) != database.ChecksumSHA256 {
			return fmt.Errorf("database %q checksum mismatch", database.Name)
		}
	}

	for i := range b.Artifacts {
		artifact := &b.Artifacts[i]
		if err := validateArtifactMetadata(
			artifact.Name, artifact.SchemaVersion, artifact.Encoding,
		); err != nil {

			return fmt.Errorf("recovery artifact %d: %w", i, err)
		}

		nameKey := strings.ToLower(artifact.Name)
		if _, ok := recoveryNames[nameKey]; ok {
			return fmt.Errorf(
				"duplicate recovery entry name %q", artifact.Name,
			)
		}
		recoveryNames[nameKey] = struct{}{}

		if len(artifact.Data) == 0 {
			return fmt.Errorf(
				"recovery artifact %q has an empty image", artifact.Name,
			)
		}
		if err := validateChecksum(artifact.ChecksumSHA256); err != nil {
			return fmt.Errorf(
				"recovery artifact %q checksum: %w", artifact.Name, err,
			)
		}
		if checksum(artifact.Data) != artifact.ChecksumSHA256 {
			return fmt.Errorf(
				"recovery artifact %q checksum mismatch", artifact.Name,
			)
		}
	}

	if err := validateChecksum(b.ChecksumSHA256); err != nil {
		return fmt.Errorf("recovery bundle checksum: %w", err)
	}
	expectedChecksum, err := checksumRecoveryBundle(b)
	if err != nil {
		return err
	}
	if expectedChecksum != b.ChecksumSHA256 {
		return errors.New("recovery bundle checksum mismatch")
	}

	return nil
}

func validateSourceConfig(config RecoveryBundleConfig) error {
	if err := validateNetwork(config.Network); err != nil {
		return err
	}
	if len(config.Databases) == 0 {
		return errors.New("at least one database source is required")
	}

	recoveryNames := make(
		map[string]struct{}, len(config.Databases)+len(config.Artifacts),
	)
	for i := range config.Databases {
		database := &config.Databases[i]
		if err := validateDatabaseMetadata(
			database.Name, database.SchemaVersion, database.Encoding,
		); err != nil {

			return fmt.Errorf("database source %d: %w", i, err)
		}

		nameKey := strings.ToLower(strings.TrimSpace(database.Name))
		if _, ok := recoveryNames[nameKey]; ok {
			return fmt.Errorf("duplicate database name %q", database.Name)
		}
		recoveryNames[nameKey] = struct{}{}

		if database.Source == nil {
			return fmt.Errorf("database source %q is nil", database.Name)
		}
	}

	for i := range config.Artifacts {
		artifact := &config.Artifacts[i]
		if err := validateArtifactMetadata(
			artifact.Name, artifact.SchemaVersion, artifact.Encoding,
		); err != nil {

			return fmt.Errorf("recovery artifact source %d: %w", i, err)
		}

		nameKey := strings.ToLower(strings.TrimSpace(artifact.Name))
		if _, ok := recoveryNames[nameKey]; ok {
			return fmt.Errorf(
				"duplicate recovery entry name %q", artifact.Name,
			)
		}
		recoveryNames[nameKey] = struct{}{}

		if artifact.Source == nil {
			return fmt.Errorf(
				"recovery artifact source %q is nil", artifact.Name,
			)
		}
	}

	return nil
}

func validateNetwork(network RecoveryNetwork) error {
	if strings.TrimSpace(network.Name) == "" {
		return errors.New("recovery network name is required")
	}
	if network.Name != strings.TrimSpace(network.Name) {
		return errors.New("recovery network name must not contain padding")
	}
	if len(network.GenesisHash) != sha256.Size*2 {
		return errors.New("recovery genesis hash must contain 64 hex digits")
	}
	if network.GenesisHash != strings.ToLower(network.GenesisHash) {
		return errors.New("recovery genesis hash must use lowercase hex")
	}
	if _, err := hex.DecodeString(network.GenesisHash); err != nil {
		return errors.New("recovery genesis hash is not valid hex")
	}

	return nil
}

func validateDatabaseMetadata(name string, schemaVersion uint32,
	encoding string) error {

	if strings.TrimSpace(name) == "" {
		return errors.New("database name is required")
	}
	if name != strings.TrimSpace(name) {
		return errors.New("database name must not contain padding")
	}
	if schemaVersion == 0 {
		return errors.New("database schema version is required")
	}
	if strings.TrimSpace(encoding) == "" {
		return errors.New("database encoding is required")
	}
	if encoding != strings.TrimSpace(encoding) {
		return errors.New("database encoding must not contain padding")
	}

	return nil
}

func validateArtifactMetadata(name string, schemaVersion uint32,
	encoding string) error {

	if strings.TrimSpace(name) == "" {
		return errors.New("recovery artifact name is required")
	}
	if name != strings.TrimSpace(name) {
		return errors.New("recovery artifact name must not contain padding")
	}
	if schemaVersion == 0 {
		return errors.New("recovery artifact schema version is required")
	}
	if strings.TrimSpace(encoding) == "" {
		return errors.New("recovery artifact encoding is required")
	}
	if encoding != strings.TrimSpace(encoding) {
		return errors.New("recovery artifact encoding must not contain padding")
	}

	return nil
}

func validateChecksum(value string) error {
	if len(value) != sha256.Size*2 {
		return errors.New("must contain 64 lowercase hex digits")
	}
	if value != strings.ToLower(value) {
		return errors.New("must contain 64 lowercase hex digits")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return errors.New("must contain 64 lowercase hex digits")
	}

	return nil
}

func checksum(value []byte) string {
	digest := sha256.Sum256(value)

	return hex.EncodeToString(digest[:])
}

func checksumRecoveryBundle(bundle *RecoveryBundle) (string, error) {
	checksumInput := struct {
		FormatVersion uint32             `json:"format_version"`
		Network       RecoveryNetwork    `json:"network"`
		CreatedAt     time.Time          `json:"created_at"`
		Databases     []DatabaseDump     `json:"databases"`
		Artifacts     []RecoveryArtifact `json:"artifacts,omitempty"`
	}{
		FormatVersion: bundle.FormatVersion,
		Network:       bundle.Network,
		CreatedAt:     bundle.CreatedAt,
		Databases:     bundle.Databases,
		Artifacts:     bundle.Artifacts,
	}

	encoded, err := json.Marshal(checksumInput)
	if err != nil {
		return "", fmt.Errorf("calculate recovery bundle checksum: %w", err)
	}

	return checksum(encoded), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode recovery bundle: trailing JSON value")
		}

		return fmt.Errorf("decode recovery bundle: %w", err)
	}

	return nil
}
