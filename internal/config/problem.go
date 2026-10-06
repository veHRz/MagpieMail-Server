package config

import (
	"cmp"
	"slices"
	"strings"
)

// Problem describes one invalid configuration setting.
type Problem struct {
	// Key is the setting's dotted path (for example "server.listen"), or the
	// environment variable name for an unknown variable.
	Key string
	// Origin is where the faulty value came from: a file path or an environment
	// variable name. It is empty for built-in defaults.
	Origin string
	// Message explains what is wrong. It never contains a secret value.
	Message string
}

func (p Problem) String() string {
	if p.Origin == "" {
		return p.Key + ": " + p.Message
	}
	return p.Key + " (from " + p.Origin + "): " + p.Message
}

// ValidationError lists every problem found while loading the configuration, so
// that an operator can fix them all in one go.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString("invalid configuration:")
	for _, p := range e.Problems {
		b.WriteString("\n  - ")
		b.WriteString(p.String())
	}
	return b.String()
}

// problems collects configuration problems and remembers which keys failed.
type problems struct {
	list   []Problem
	failed map[string]bool
}

func (ps *problems) add(key, origin, message string) {
	if ps.failed == nil {
		ps.failed = make(map[string]bool)
	}
	ps.failed[key] = true
	ps.list = append(ps.list, Problem{Key: key, Origin: origin, Message: message})
}

func (ps *problems) has(key string) bool { return ps.failed[key] }

func (ps *problems) err() error {
	if len(ps.list) == 0 {
		return nil
	}
	sorted := slices.SortedStableFunc(slices.Values(ps.list), func(a, b Problem) int {
		return cmp.Compare(a.Key, b.Key)
	})
	return &ValidationError{Problems: sorted}
}
