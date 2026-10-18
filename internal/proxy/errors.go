package proxy

import "errors"

var (
	ErrTooManyHops              = errors.New("proxy: Max-Forwards reached zero")
	ErrInvalidMaxForwards       = errors.New("proxy: invalid Max-Forwards")
	ErrMissingTransactionHeader = errors.New("proxy: missing header needed for the branch")
)
