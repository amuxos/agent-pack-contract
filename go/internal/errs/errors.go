// Package errs defines the shared error type for agent-pack-contract.
package errs

import "fmt"

// ContractError is a user-facing error that the CLI prints as "ERROR: <msg>"
// and exits 1 on.
type ContractError struct {
	Msg string
}

func (e *ContractError) Error() string { return e.Msg }

// New builds a ContractError with fmt.Sprintf semantics.
func New(format string, args ...any) *ContractError {
	return &ContractError{Msg: fmt.Sprintf(format, args...)}
}
