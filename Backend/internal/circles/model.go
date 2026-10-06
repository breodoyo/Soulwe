package circles

import (
	"errors"
	"time"
)

const (
	// MaxMessageLength caps a single circle message at 1,000 Unicode code points.
	MaxMessageLength = 1000
)

// DefaultAuthorName labels a member with no display_name; the email local part is never published.
const DefaultAuthorName = "Member"

// Shared list pagination defaults (default 20, maximum 50) for the authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// ClampLimit falls back to the default for non-positive limits and caps large ones.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

// Sentinel errors mapped to safe HTTP responses; none embed user content, identities, or secrets.
var (
	// ErrCircleNotFound reports a lookup or join/leave that matched no active circle.
	ErrCircleNotFound = errors.New("circle not found")
	// ErrAlreadyMember reports a join by an identity that is already a member.
	ErrAlreadyMember = errors.New("already a member of this circle")
	// ErrNotMember reports message reads/writes by an identity that has not joined.
	ErrNotMember = errors.New("not a member of this circle")
	// ErrInvalidContent reports empty or over-long message content.
	ErrInvalidContent = errors.New("invalid circle message content")
)

// Circle is the wire shape for discovery/detail; no owner identity ID is exposed.
type Circle struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Icon        *string   `json:"icon"`
	MemberCount int       `json:"member_count"`
	IsMember    bool      `json:"is_member"`
	CreatedAt   time.Time `json:"-"`
}

// CircleMessage is the wire shape for a circle message. AuthorName is the
// author's display_name when registered or their pseudonym when anonymous;
// IsAnonymous says which. Neither owner ID is ever serialized.
type CircleMessage struct {
	ID             string         `json:"id"`
	AuthorName     string         `json:"author_name"`
	IsAnonymous    bool           `json:"is_anonymous"`
	Content        string         `json:"content"`
	ReactionCounts map[string]int `json:"reaction_counts"`
	CreatedAt      time.Time      `json:"created_at"`
}
