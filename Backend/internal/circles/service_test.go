package circles

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	circleA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	circleB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	circleX = "99999999-9999-9999-9999-999999999999"
	anonOne = "11111111-1111-1111-1111-111111111111"
	anonTwo = "22222222-2222-2222-2222-222222222222"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func strPtr(s string) *string { return &s }

// fakeRepository is an in-memory Repository. Circles, memberships, and
// messages are keyed by UUID so the service's wording can be asserted without
// a database.
type fakeRepository struct {
	mu         sync.Mutex
	circles    map[string]*Circle
	members    map[string]map[string]bool // circleID -> identityID -> joined
	messages   map[string][]CircleMessage // circleID -> newest-last
	anonNames  map[string]string
	messageSeq int
	lastLimit  int // last limit requested by the service, for clamp assertions
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		circles: map[string]*Circle{
			circleA: {ID: circleA, Slug: "grief", Name: "Grief & Loss", Description: strPtr("grief support"), Icon: strPtr("🌿"), CreatedAt: testNow},
			circleB: {ID: circleB, Slug: "family", Name: "Family", Description: nil, Icon: strPtr("🏠"), CreatedAt: testNow},
		},
		members:   map[string]map[string]bool{},
		messages:  map[string][]CircleMessage{},
		anonNames: map[string]string{anonOne: "Anon Baobab", anonTwo: "Anon Willow"},
	}
}

func (f *fakeRepository) ListCircles(ctx context.Context) ([]Circle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Circle, 0, len(f.circles))
	for _, c := range f.circles {
		cp := *c
		cp.MemberCount = len(f.members[c.ID])
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeRepository) GetCircle(ctx context.Context, circleID string) (*Circle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.circles[circleID]
	if !ok {
		return nil, ErrCircleNotFound
	}
	cp := *c
	cp.MemberCount = len(f.members[circleID])
	return &cp, nil
}

func (f *fakeRepository) IsMember(ctx context.Context, circleID, anonIdentityID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.members[circleID][anonIdentityID], nil
}

func (f *fakeRepository) AddMember(ctx context.Context, circleID, anonIdentityID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.members[circleID] == nil {
		f.members[circleID] = map[string]bool{}
	}
	if f.members[circleID][anonIdentityID] {
		return ErrAlreadyMember
	}
	f.members[circleID][anonIdentityID] = true
	return nil
}

func (f *fakeRepository) RemoveMember(ctx context.Context, circleID, anonIdentityID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.members[circleID], anonIdentityID)
	return nil
}

func (f *fakeRepository) ListMessages(ctx context.Context, circleID string, limit int, before *time.Time) ([]CircleMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastLimit = limit
	all := f.messages[circleID]
	out := make([]CircleMessage, 0)
	for i := len(all) - 1; i >= 0; i-- {
		m := all[i]
		if before != nil && !m.CreatedAt.Before(*before) {
			continue
		}
		cp := m
		out = append(out, cp)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepository) CreateMessage(ctx context.Context, circleID, anonIdentityID, content string) (*CircleMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messageSeq++
	m := CircleMessage{
		ID:             fmt.Sprintf("msg-%d", f.messageSeq),
		AnonName:       f.anonNames[anonIdentityID],
		Content:        content,
		ReactionCounts: map[string]int{},
		CreatedAt:      testNow.Add(time.Duration(f.messageSeq) * time.Second),
	}
	f.messages[circleID] = append(f.messages[circleID], m)
	return &m, nil
}

func TestService_List(t *testing.T) {
	svc := NewService(newFakeRepository())

	circles, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(circles) != 2 {
		t.Fatalf("expected 2 circles, got %d", len(circles))
	}
	for _, c := range circles {
		if c.MemberCount != 0 {
			t.Errorf("expected zero member count before any joins, got %d for %s", c.MemberCount, c.Name)
		}
		if c.IsMember {
			t.Errorf("discovery must not report is_member for %s", c.Name)
		}
	}
}

func TestService_Get(t *testing.T) {
	svc := NewService(newFakeRepository())
	ctx := context.Background()

	if err := svc.Join(ctx, anonOne, circleA); err != nil {
		t.Fatalf("setup join: %v", err)
	}

	t.Run("reports membership for the caller", func(t *testing.T) {
		circle, err := svc.Get(ctx, anonOne, circleA)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if !circle.IsMember {
			t.Error("expected is_member true for the identity that joined")
		}
		if circle.MemberCount != 1 {
			t.Errorf("expected member count 1, got %d", circle.MemberCount)
		}
	})

	t.Run("reports no membership for others", func(t *testing.T) {
		circle, err := svc.Get(ctx, anonTwo, circleA)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if circle.IsMember {
			t.Error("expected is_member false for an identity that never joined")
		}
		if circle.MemberCount != 1 {
			t.Errorf("member count must stay global: expected 1, got %d", circle.MemberCount)
		}
	})

	t.Run("missing circle is a not-found error", func(t *testing.T) {
		if _, err := svc.Get(ctx, anonOne, circleX); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
	})
}

func TestService_JoinAndLeave(t *testing.T) {
	svc := NewService(newFakeRepository())
	ctx := context.Background()

	t.Run("join then duplicate join conflicts", func(t *testing.T) {
		if err := svc.Join(ctx, anonOne, circleA); err != nil {
			t.Fatalf("first join: %v", err)
		}
		if err := svc.Join(ctx, anonOne, circleA); !errors.Is(err, ErrAlreadyMember) {
			t.Errorf("expected ErrAlreadyMember on a duplicate join, got %v", err)
		}
	})

	t.Run("join on a missing circle is a not-found error", func(t *testing.T) {
		if err := svc.Join(ctx, anonOne, circleX); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
	})

	t.Run("leave is idempotent", func(t *testing.T) {
		other := NewService(newFakeRepository())
		if err := other.Leave(ctx, anonTwo, circleA); err != nil {
			t.Fatalf("leaving a circle never joined must succeed, got %v", err)
		}
	})

	t.Run("leave on a missing circle is a not-found error", func(t *testing.T) {
		if err := svc.Leave(ctx, anonOne, circleX); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
	})

	t.Run("member count updates across identities", func(t *testing.T) {
		if err := svc.Join(ctx, anonTwo, circleA); err != nil {
			t.Fatalf("second member join: %v", err)
		}
		circle, err := svc.Get(ctx, anonOne, circleA)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if circle.MemberCount != 2 {
			t.Errorf("expected 2 members after both joined, got %d", circle.MemberCount)
		}
		if err := svc.Leave(ctx, anonTwo, circleA); err != nil {
			t.Fatalf("leave: %v", err)
		}
		after, err := svc.Get(ctx, anonOne, circleA)
		if err != nil {
			t.Fatalf("Get after leave: %v", err)
		}
		if after.MemberCount != 1 {
			t.Errorf("expected 1 member after one left, got %d", after.MemberCount)
		}
	})
}

func TestService_SendAndListMessages(t *testing.T) {
	svc := NewService(newFakeRepository())
	ctx := context.Background()

	if err := svc.Join(ctx, anonOne, circleA); err != nil {
		t.Fatalf("setup join: %v", err)
	}

	t.Run("non-members cannot send or read", func(t *testing.T) {
		if _, err := svc.SendMessage(ctx, anonTwo, circleA, "private"); !errors.Is(err, ErrNotMember) {
			t.Errorf("expected ErrNotMember sending, got %v", err)
		}
		if _, err := svc.ListMessages(ctx, anonTwo, circleA, 10, nil); !errors.Is(err, ErrNotMember) {
			t.Errorf("expected ErrNotMember reading, got %v", err)
		}
	})

	t.Run("missing circle is a not-found error, not a membership error", func(t *testing.T) {
		if _, err := svc.SendMessage(ctx, anonOne, circleX, "hello"); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
		if _, err := svc.ListMessages(ctx, anonOne, circleX, 10, nil); !errors.Is(err, ErrCircleNotFound) {
			t.Errorf("expected ErrCircleNotFound, got %v", err)
		}
	})

	t.Run("invalid content is rejected", func(t *testing.T) {
		if _, err := svc.SendMessage(ctx, anonOne, circleA, "   "); !errors.Is(err, ErrInvalidContent) {
			t.Errorf("expected ErrInvalidContent for blank content, got %v", err)
		}
		if _, err := svc.SendMessage(ctx, anonOne, circleA, stringOfRunes(MaxMessageLength+1, 'x')); !errors.Is(err, ErrInvalidContent) {
			t.Errorf("expected ErrInvalidContent for over-long content, got %v", err)
		}
	})

	t.Run("a member sends and reads messages newest first", func(t *testing.T) {
		first, err := svc.SendMessage(ctx, anonOne, circleA, "Lost my father last month.")
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if first.ID == "" || first.AnonName == "" {
			t.Errorf("expected id and anon_name on the created message, got %+v", first)
		}
		if first.AnonName != "Anon Baobab" {
			t.Errorf("expected the creator's anon_name, got %q", first.AnonName)
		}
		second, err := svc.SendMessage(ctx, anonOne, circleA, "It is so hard.")
		if err != nil {
			t.Fatalf("second SendMessage: %v", err)
		}
		if second.CreatedAt.Before(first.CreatedAt) {
			t.Error("second message must be newer than the first")
		}

		messages, err := svc.ListMessages(ctx, anonOne, circleA, 10, nil)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
		if messages[0].ID != second.ID {
			t.Errorf("expected the newest message first, got %s", messages[0].ID)
		}
		for _, m := range messages {
			if m.AnonName == "" || m.ReactionCounts == nil {
				t.Errorf("message missing anon_name or a non-nil reaction_counts: %+v", m)
			}
		}
	})

	t.Run("pagination honors before and clamps limit", func(t *testing.T) {
		fake := newFakeRepository()
		svc := NewService(fake)
		if err := svc.Join(ctx, anonOne, circleA); err != nil {
			t.Fatalf("setup join: %v", err)
		}
		for i := 0; i < 5; i++ {
			if _, err := svc.SendMessage(ctx, anonOne, circleA, fmt.Sprintf("note %d", i)); err != nil {
				t.Fatalf("SendMessage: %v", err)
			}
		}
		all, err := svc.ListMessages(ctx, anonOne, circleA, 1000, nil)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if len(all) != 5 {
			t.Fatalf("expected 5 messages after seeding, got %d", len(all))
		}

		if _, err := svc.ListMessages(ctx, anonOne, circleA, 10_000, nil); err != nil {
			t.Fatalf("ListMessages clamped: %v", err)
		}
		if fake.lastLimit != MaxListLimit {
			t.Errorf("expected the repository to receive the clamped limit %d, got %d", MaxListLimit, fake.lastLimit)
		}

		cursor := all[len(all)-3].CreatedAt
		paged, err := svc.ListMessages(ctx, anonOne, circleA, 1000, &cursor)
		if err != nil {
			t.Fatalf("ListMessages paged: %v", err)
		}
		if len(paged) != 2 {
			t.Errorf("expected 2 messages older than the cursor, got %d", len(paged))
		}
		for _, m := range paged {
			if !m.CreatedAt.Before(cursor) {
				t.Errorf("pagination returned a message not older than the cursor")
			}
		}
	})

	t.Run("a member can read alone and empty circle lists are non-nil", func(t *testing.T) {
		if err := svc.Join(ctx, anonTwo, circleB); err != nil {
			t.Fatalf("join circle B: %v", err)
		}
		messages, err := svc.ListMessages(ctx, anonTwo, circleB, 10, nil)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		if messages == nil || len(messages) != 0 {
			t.Errorf("expected a non-nil empty slice, got %#v", messages)
		}
	})
}

func stringOfRunes(n int, r rune) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = r
	}
	return string(b)
}
