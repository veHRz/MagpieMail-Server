package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/veHRz/MagpieMail-Server/internal/store/postgres/gen"
)

// AuditEvent is a security-relevant event. Details must never hold message
// content, tokens or passwords.
type AuditEvent struct {
	ID          uuid.UUID
	OccurredAt  time.Time
	ActorUserID *uuid.UUID // nil for the system
	Action      string     // dotted, e.g. "auth.login_failed"
	TargetType  string
	TargetID    string
	ClientIP    *netip.Addr
	Details     map[string]any
}

// AuditFilter selects audit events, newest first.
type AuditFilter struct {
	Actor  *uuid.UUID // nil for every actor
	Before time.Time  // zero for now
	Limit  int32
}

// Audit is the repository of the audit log: append and read, never update.
type Audit struct{ s *Store }

// Audit returns the audit log repository.
func (s *Store) Audit() Audit { return Audit{s} }

// Append records an event. ID and OccurredAt are set when zero.
func (r Audit) Append(ctx context.Context, e AuditEvent) error {
	if e.ID == uuid.Nil {
		e.ID = newID()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}
	details := []byte("{}")
	if len(e.Details) > 0 {
		var err error
		if details, err = json.Marshal(e.Details); err != nil {
			return fmt.Errorf("encoding audit details: %w", err)
		}
	}
	return translate(r.s.q.AppendAuditEvent(ctx, gen.AppendAuditEventParams{
		ID: e.ID, OccurredAt: e.OccurredAt.UTC(), ActorUserID: e.ActorUserID, Action: e.Action,
		TargetType: e.TargetType, TargetID: e.TargetID, ClientIp: e.ClientIP, Details: details,
	}))
}

// List returns events matching the filter, newest first.
func (r Audit) List(ctx context.Context, f AuditFilter) ([]AuditEvent, error) {
	if f.Before.IsZero() {
		f.Before = time.Now().Add(time.Second)
	}
	rows, err := r.s.q.ListAuditEvents(ctx, gen.ListAuditEventsParams{Actor: f.Actor, Before: f.Before, MaxEvents: f.Limit})
	if err != nil {
		return nil, translate(err)
	}
	events := make([]AuditEvent, 0, len(rows))
	for _, row := range rows {
		var details map[string]any
		if err := json.Unmarshal(row.Details, &details); err != nil {
			return nil, fmt.Errorf("decoding audit details: %w", err)
		}
		events = append(events, AuditEvent{
			ID: row.ID, OccurredAt: row.OccurredAt, ActorUserID: row.ActorUserID, Action: row.Action,
			TargetType: row.TargetType, TargetID: row.TargetID, ClientIP: row.ClientIp, Details: details,
		})
	}
	return events, nil
}
