package extract

import "errors"

var (
	// ErrInvalidDocument indicates that input could not be decoded as the
	// expected document format.
	ErrInvalidDocument = errors.New("input is not a valid document")

	// ErrEncryptedDocument indicates that input requires unsupported
	// decryption or password handling.
	ErrEncryptedDocument = errors.New("encrypted documents are not supported")
)
