// Package buildinfo exposes the version stamped into the binary at build time:
//
//	go build -ldflags "-X github.com/veHRz/MagpieMail-Server/internal/buildinfo.Version=v1.2.3 \
//	  -X github.com/veHRz/MagpieMail-Server/internal/buildinfo.Commit=abc1234"
package buildinfo

var (
	// Version is the semantic version of the build, or "dev".
	Version = "dev"
	// Commit is the git commit of the build, or "unknown".
	Commit = "unknown"
)
