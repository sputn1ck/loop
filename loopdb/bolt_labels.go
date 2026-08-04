//go:build !js || !wasm

package loopdb

import (
	"github.com/lightninglabs/loop/labels"
	"go.etcd.io/bbolt"
)

// putLabel performs validation of a label and writes it to the bucket provided
// under the label key if it is non-zero.
func putLabel(bucket *bbolt.Bucket, label string) error {
	if len(label) == 0 {
		return nil
	}

	// Check that the label does not exceed our maximum length.
	if len(label) > labels.MaxLength {
		return labels.ErrLabelTooLong
	}

	return bucket.Put(labelKey, []byte(label))
}

// getLabel attempts to get an optional label stored under the label key in a
// bucket. If it is not present, an empty label is returned.
func getLabel(bucket *bbolt.Bucket) string {
	label := bucket.Get(labelKey)
	if label == nil {
		return ""
	}

	return string(label)
}
