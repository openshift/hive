// Package store defines generic interfaces for signature stores.
package store

import (
	"context"
)

// Callback returns true if an acceptable signature has been found, or
// an error if the loop should be aborted.  If there was a problem
// retrieving the signature, the incoming error will describe the
// problem and the function can decide how to handle that error.
type Callback func(ctx context.Context, signature []byte, errIn error) (done bool, err error)

// Store provides access to signatures by digest.
type Store interface {

	// Signatures fetches signatures for the provided digest, feeding
	// them into the provided callback until an acceptable signature is
	// found or an error occurs.  Not finding any acceptable signatures
	// is not an error; it is up to the caller to handle that case.
	Signatures(ctx context.Context, name string, digest string, fn Callback) error

	// String returns a description of where this store finds
	// signatures.  The string is a short clause intended for display in
	// a description of the verifier.
	String() string
}
