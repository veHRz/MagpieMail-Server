package httpapi

// NewChainForTest exposes the shared middleware chain to the external tests,
// which mount test-only operations on it.
var NewChainForTest = newChain
