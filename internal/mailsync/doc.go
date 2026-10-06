// Package mailsync synchronizes accounts with their providers: initial and
// incremental sync, IDLE, and the queue of actions replayed on the provider.
//
// It is not named "sync" to avoid shadowing the standard library package.
package mailsync
