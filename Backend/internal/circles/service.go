package circles

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// Service is the circles business-logic boundary. It owns the input
// validation, the membership gate (message reads/writes require membership),
// and the identity plumbing — every operation is scoped to the authenticated
// anonymous identity and never accepts one from a request body. It never
// constructs SQL and never exposes anon_identity_ids, device UUIDs, or token
// hashes on the wire.
type Service interface {
	// List returns all active circles with live member counts, ordered by name.
	List(ctx context.Context) ([]Circle, error)

	// Get returns one circle with its member count and whether the identity
	// has joined. Returns ErrCircleNotFound for a missing or inactive circle.
	Get(ctx context.Context, anonIdentityID, circleID string) (*Circle, error)

	// Join adds the identity to the circle. Returns ErrCircleNotFound when
	// the circle is missing or inactive and ErrAlreadyMember on a duplicate.
	Join(ctx context.Context, anonIdentityID, circleID string) error

	// Leave removes the identity's membership. Leaving a circle the identity
	// never joined is a no-op success; ErrCircleNotFound when the circle is
	// missing or inactive.
	Leave(ctx context.Context, anonIdentityID, circleID string) error

	// ListMessages returns the circle's messages newest first, clamped to a
	// sane page size, optionally resuming from a created_at cursor. Members
	// only: returns ErrNotMember (or ErrCircleNotFound) to non-members.
	ListMessages(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error)

	// SendMessage validates content, verifies membership, and stores the
	// message authored by the identity. Returns ErrCircleNotFound,
	// ErrNotMember, or ErrInvalidContent.
	SendMessage(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error)
}

type service struct {
	circles Repository
}

// NewService wires the circles service to a repository.
func NewService(circles Repository) *service {
	return &service{circles: circles}
}

func (s *service) List(ctx context.Context) ([]Circle, error) {
	circles, err := s.circles.ListCircles(ctx)
	if err != nil {
		return nil, err
	}
	// Keep slices non-nil so the wire shape is always {"circles":[]}.
	if circles == nil {
		circles = make([]Circle, 0)
	}
	return circles, nil
}

func (s *service) Get(ctx context.Context, anonIdentityID, circleID string) (*Circle, error) {
	circle, err := s.circles.GetCircle(ctx, circleID)
	if err != nil {
		return nil, err
	}
	isMember, err := s.circles.IsMember(ctx, circleID, anonIdentityID)
	if err != nil {
		return nil, err
	}
	circle.IsMember = isMember
	return circle, nil
}

func (s *service) Join(ctx context.Context, anonIdentityID, circleID string) error {
	if _, err := s.circles.GetCircle(ctx, circleID); err != nil {
		return err
	}
	return s.circles.AddMember(ctx, circleID, anonIdentityID)
}

func (s *service) Leave(ctx context.Context, anonIdentityID, circleID string) error {
	if _, err := s.circles.GetCircle(ctx, circleID); err != nil {
		return err
	}
	return s.circles.RemoveMember(ctx, circleID, anonIdentityID)
}

func (s *service) ListMessages(ctx context.Context, anonIdentityID, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
	if err := s.requireMember(ctx, anonIdentityID, circleID); err != nil {
		return nil, err
	}
	messages, err := s.circles.ListMessages(ctx, circleID, ClampLimit(limit), before)
	if err != nil {
		return nil, err
	}
	if messages == nil {
		messages = make([]CircleMessage, 0)
	}
	for i := range messages {
		normalizeMessage(&messages[i])
	}
	return messages, nil
}

func (s *service) SendMessage(ctx context.Context, anonIdentityID, circleID, content string) (*CircleMessage, error) {
	content = strings.TrimSpace(content)
	if err := validateContent(content); err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, anonIdentityID, circleID); err != nil {
		return nil, err
	}
	message, err := s.circles.CreateMessage(ctx, circleID, anonIdentityID, content)
	if err != nil {
		return nil, err
	}
	normalizeMessage(message)
	return message, nil
}

// requireMember gates message reads/writes behind membership. A missing or
// inactive circle surfaces as ErrCircleNotFound (404); an identity that has
// not joined surfaces as ErrNotMember (403).
func (s *service) requireMember(ctx context.Context, anonIdentityID, circleID string) error {
	isMember, err := s.circles.IsMember(ctx, circleID, anonIdentityID)
	if err != nil {
		return err
	}
	if isMember {
		return nil
	}
	// Distinguish "this circle does not exist" from "you are not a member".
	if _, err := s.circles.GetCircle(ctx, circleID); err != nil {
		return err
	}
	return ErrNotMember
}

func validateContent(content string) error {
	if content == "" {
		return ErrInvalidContent
	}
	if utf8.RuneCountInString(content) > MaxMessageLength {
		return ErrInvalidContent
	}
	return nil
}

// normalizeMessage ensures reaction_counts is the documented empty object
// rather than JSON null when a row predates any reactions.
func normalizeMessage(m *CircleMessage) {
	if m.ReactionCounts == nil {
		m.ReactionCounts = map[string]int{}
	}
}

// ensure the concrete service satisfies the interface.
var _ Service = (*service)(nil)
