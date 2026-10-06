// Package domain holds the entities and business rules of MagpieMail.
//
// It has no I/O dependencies: no database, network, file system or clock
// access. Other layers depend on it, never the reverse. The rule is enforced by
// depguard in .golangci.yml.
package domain
