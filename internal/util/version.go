package util

// Version is the application version. It defaults to "development" and
// can be overridden at build time via:
//
//	go build -ldflags "-X github.com/mxcd/handoff/internal/util.Version=v1.0.0"
var Version = "development"

// Commit is the git commit hash. It defaults to "unknown" and
// can be overridden at build time via:
//
//	go build -ldflags "-X github.com/mxcd/handoff/internal/util.Commit=abc1234"
var Commit = "unknown"
