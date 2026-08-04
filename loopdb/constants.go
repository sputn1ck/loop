package loopdb

import (
	"encoding/binary"
	"time"
)

var byteOrder = binary.BigEndian

const (
	// keyLength is the length of a serialized public key.
	keyLength = 33

	// DefaultLoopOutHtlcConfirmations is the default number of
	// confirmations we set for a loop out htlc.
	DefaultLoopOutHtlcConfirmations uint32 = 1

	// DefaultLoopDBTimeout is the default maximum time we wait for the
	// Loop bbolt database to be opened.
	DefaultLoopDBTimeout = 5 * time.Second
)
