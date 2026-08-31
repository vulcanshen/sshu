package remote

import (
	"errors"

	"golang.org/x/crypto/ssh/knownhosts"
)

// asKeyError is errors.As narrowed to *knownhosts.KeyError, kept out of sftp.go
// so the verification logic there reads as policy rather than plumbing.
func asKeyError(err error, target **knownhosts.KeyError) bool {
	return errors.As(err, target)
}
