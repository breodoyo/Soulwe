package circles

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"Backend/internal/middleware"
)

type Service interface {
	List(ctx context.Context) ([]Circle, error)

	Get(ctx context.Context, owner middleware.Owner, circleID string) (*Circle, error)

	Join(ctx context.Context, owner middleware.Owner, circleID string) error

	Leave(ctx context.Context, owner middleware.Owner, circleID string) error

	ListMessages(ctx context.Context, owner middleware.Owner, circleID string, limit int, before *time.Time) ([]CircleMessage, error)

	SendMessage(ctx context.Context, owner middleware.Owner, circleID, content string) (*CircleMessage, error)
}

type service struct {
	circles Repository
	names   nameDeriver
}

// NewService wires the circle domain with the secret that derives per-circle
// display names. The key must be server-side only: it is what keeps a label
// reversible to nothing.
func NewService(circles Repository, namingKey string) *service {
	return &service{circles: circles, names: newNameDeriver(namingKey)}
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

func (s *service) Get(ctx context.Context, owner middleware.Owner, circleID string) (*Circle, error) {
	circle, err := s.circles.GetCircle(ctx, circleID)
	if err != nil {
		return nil, err
	}
	isMember, err := s.circles.IsMember(ctx, circleID, owner)
	if err != nil {
		return nil, err
	}
	circle.IsMember = isMember
	return circle, nil
}

func (s *service) Join(ctx context.Context, owner middleware.Owner, circleID string) error {
	if _, err := s.circles.GetCircle(ctx, circleID); err != nil {
		return err
	}
	return s.circles.AddMember(ctx, circleID, owner)
}

func (s *service) Leave(ctx context.Context, owner middleware.Owner, circleID string) error {
	if _, err := s.circles.GetCircle(ctx, circleID); err != nil {
		return err
	}
	return s.circles.RemoveMember(ctx, circleID, owner)
}

func (s *service) ListMessages(ctx context.Context, owner middleware.Owner, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
	if err := s.requireMember(ctx, owner, circleID); err != nil {
		return nil, err
	}
	messages, err := s.circles.ListMessages(ctx, circleID, ClampLimit(limit), before)
	if err != nil {
		return nil, err
	}
	if messages == nil {
		messages = make([]CircleMessage, 0)
	}
	viewer, _ := middleware.IdentityFromOwner(owner)
	for i := range messages {
		s.labelMessage(&messages[i], viewer, circleID)
	}
	return messages, nil
}

func (s *service) SendMessage(ctx context.Context, owner middleware.Owner, circleID, content string) (*CircleMessage, error) {
	content = strings.TrimSpace(content)
	if err := validateContent(content); err != nil {
		return nil, err
	}
	if err := s.requireMember(ctx, owner, circleID); err != nil {
		return nil, err
	}
	message, err := s.circles.CreateMessage(ctx, circleID, owner, content)
	if err != nil {
		return nil, err
	}
	// The author is the caller, so the created message is always their own.
	if identity, ok := middleware.IdentityFromOwner(owner); ok {
		message.AuthorName = s.names.derive(identity, circleID)
	}
	message.IsMine = true
	normalizeMessage(message)
	return message, nil
}

// labelMessage replaces whatever label the repository resolved with the
// per-circle one and records whether the message belongs to the reader, so the
// UI can mark "You" without any identity leaving the API.
func (s *service) labelMessage(m *CircleMessage, viewerIdentity, circleID string) {
	if m.AuthorIdentityID != "" {
		m.AuthorName = s.names.derive(m.AuthorIdentityID, circleID)
		m.IsMine = m.AuthorIdentityID == viewerIdentity && viewerIdentity != ""
	}
	normalizeMessage(m)
}

func (s *service) requireMember(ctx context.Context, owner middleware.Owner, circleID string) error {
	isMember, err := s.circles.IsMember(ctx, circleID, owner)
	if err != nil {
		return err
	}
	if isMember {
		return nil
	}

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

func normalizeMessage(m *CircleMessage) {
	if m.ReactionCounts == nil {
		m.ReactionCounts = map[string]int{}
	}
	if strings.TrimSpace(m.AuthorName) == "" {
		m.AuthorName = DefaultAuthorName
	}
}

var _ Service = (*service)(nil)
