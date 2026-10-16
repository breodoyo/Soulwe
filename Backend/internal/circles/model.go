package circles

import (
	"errors"
	"time"
)

const (
	MaxMessageLength = 1000
)

const DefaultAuthorName = "Member"

const (
	DefaultListLimit = 20
	MaxListLimit     = 50
)

func ClampLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

var (
	
	ErrCircleNotFound = errors.New("circle not found")
	ErrAlreadyMember = errors.New("already a member of this circle")
	ErrNotMember = errors.New("not a member of this circle")
	ErrInvalidContent = errors.New("invalid circle message content")
)

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

type CircleMessage struct {
	ID             string         `json:"id"`
	AuthorName     string         `json:"author_name"`
	IsAnonymous    bool           `json:"is_anonymous"`
	Content        string         `json:"content"`
	ReactionCounts map[string]int `json:"reaction_counts"`
	CreatedAt      time.Time      `json:"created_at"`
}
