// Package egress is the only way out to the network.
//
// Every outbound connection (mail providers, external AI, image proxy, Web Push,
// key discovery, webhooks) goes through it, so that it is logged, can be
// disabled per feature, and can be routed through an upstream proxy or VPN.
// Outbound network APIs are forbidden elsewhere by forbidigo in .golangci.yml.
package egress
