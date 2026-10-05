package circles

import (
	"errors"
	"time"
)

// Content and pagination limits for the circles domain. These are
// application-level guards; the database schema stays unchanged.
const (
	// MaxMessageLength caps a single circle message at 1,000 Unicode code
	// points — long enough for a heartfelt note, short enough to keep the
	// anonymous rooms readable.
	MaxMessageLength = 1000
)

// DefaultAuthorName labels a registered member who has not chosen a display
// name. users.display_name is nullable, so a real account can reach a circle
// without one; rather than fall back to the email local part — which would
// publish a contact address to every member of the room — it gets the same
// neutral label the rest of the app uses for an unnamed account.
const DefaultAuthorName = "Member"

// List pagination defaults, matching the documented convention (default 20,
// maximum 50) shared with the other authenticated collections.
const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

// ClampLimit applies the documented page-size defaults/ceiling to a raw
// client-supplied limit. Non-positive values fall back to the default; large
// values are capped rather than rejected. Exported so the handler can compute
// the next_cursor from the effective page size.
func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

// Sentinel errors returned by the circles domain. Handlers map these to safe
// HTTP responses; none of them embed user content, identities, or secrets.
var (
	// ErrCircleNotFound reports a lookup (or join/leave) that matched no
	// active circle.
	ErrCircleNotFound = errors.New("circle not found")
	// ErrAlreadyMember reports a join by an identity that is already a member.
	ErrAlreadyMember = errors.New("already a member of this circle")
	// ErrNotMember reports message reads/writes from an identity that has not
	// joined the circle.
	ErrNotMember = errors.New("not a member of this circle")
	// ErrInvalidContent reports empty or over-long message content.
	ErrInvalidContent = errors.New("invalid circle message content")
)

// Circle mirrors the circles table with its live member_count rolled up. It is
// the wire shape for discovery and detail responses. AnonIdentityID never
// appears; circles are public rooms, memberships are private to the caller.
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

// CircleMessage is the wire shape for a circle chat message.
//
// Circles accept both kinds of owner, so an author is described by a resolved
// display name rather than by an identity kind. AuthorName is the author's own
// display_name when they posted as a registered member, and their
// server-generated pseudonym when they posted anonymously; IsAnonymous says
// which, so a client can label the two without inferring it.
//
// Neither owner ID is ever serialized: user_id, anon_identity_id, device UUIDs
// and token hashes stay on the server, and a registered member's identity is
// only ever visible as the name they chose to publish.
type CircleMessage struct {
	ID             string         `json:"id"`
	AuthorName     string         `json:"author_name"`
	IsAnonymous    bool           `json:"is_anonymous"`
	Content        string         `json:"content"`
	ReactionCounts map[string]int `json:"reaction_counts"`
	CreatedAt      time.Time      `json:"created_at"`
}
