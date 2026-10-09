// Package version holds the agent-pack-contract version.
//
// This is the canonical release version.
//
// Overridable at build time via ldflags:
//
//	go build -ldflags "-X github.com/amuxos/agent-pack-contract/go/internal/version.Version=vX.Y.Z"
package version

var Version = "0.8.2"
