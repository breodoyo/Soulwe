package anon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeRepository is an in-memory, concurrency-safe Repository that mimics the
// real table's UNIQUE constraints on anon_name, token_hash and device_uuid.
type fakeRepository struct {
	mu           sync.Mutex
	byTokenHash  map[string]*AnonIdentity
	byDeviceUUID map[string]*AnonIdentity
	takenNames   map[string]bool
	nextID       int
	lastSeenAt   map[string]bool
	creates      int
	conflicts    int // upcoming Create calls that should collide on name/token
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		byTokenHash:  map[string]*AnonIdentity{},
		byDeviceUUID: map[string]*AnonIdentity{},
		takenNames:   map[string]bool{},
		lastSeenAt:   map[string]bool{},
	}
}

func (f *fakeRepository) Create(_ context.Context, identity *AnonIdentity) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.conflicts > 0 {
		f.conflicts--
		return ErrIdentityConflict
	}
	if f.byTokenHash[identity.TokenHash] != nil || f.takenNames[identity.AnonName] {
		return ErrIdentityConflict
	}
	if identity.DeviceUUID != nil && f.byDeviceUUID[*identity.DeviceUUID] != nil {
		return ErrDeviceConflict
	}

	f.nextID++
	identity.ID = string(rune('a'+f.nextID)) + strings.Repeat("0", 35)
	f.takenNames[identity.AnonName] = true
	f.byTokenHash[identity.TokenHash] = identity
	if identity.DeviceUUID != nil {
		f.byDeviceUUID[*identity.DeviceUUID] = identity
	}
	f.creates++
	return nil
}

func (f *fakeRepository) FindByTokenHash(_ context.Context, tokenHash string) (*AnonIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	identity, ok := f.byTokenHash[tokenHash]
	if !ok {
		return nil, ErrIdentityNotFound
	}
	return identity, nil
}

func (f *fakeRepository) UpdateLastSeen(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSeenAt[id] = true
	return nil
}

func TestServiceCreateSession(t *testing.T) {
	t.Run("returns a fresh token and identity id", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		session, err := svc.CreateSession(context.Background(), "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if session.Token == "" {
			t.Error("expected a non-empty raw token")
		}
		if session.AnonymousID == "" {
			t.Error("expected a non-empty anonymous id")
		}
		if repo.creates != 1 {
			t.Errorf("expected exactly one create, got %d", repo.creates)
		}
		if _, ok := repo.byTokenHash[session.Token]; ok {
			t.Error("the raw token was stored as the token_hash — only the hash should be stored")
		}
		if _, ok := repo.byTokenHash[HashToken(session.Token)]; !ok {
			t.Error("expected the SHA-256 hash of the token to be stored")
		}
	})

	t.Run("two concurrent calls both succeed with different identities", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		const calls = 2
		sessions := make([]*Session, calls)
		errs := make([]error, calls)

		var wg sync.WaitGroup
		for i := 0; i < calls; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sessions[i], errs[i] = svc.CreateSession(context.Background(), "")
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("concurrent call %d returned error: %v", i, err)
			}
		}
		if sessions[0].AnonymousID == sessions[1].AnonymousID {
			t.Errorf("concurrent calls shared an identity id %q", sessions[0].AnonymousID)
		}
		if sessions[0].Token == sessions[1].Token {
			t.Error("concurrent calls shared a token")
		}
		if repo.creates != calls {
			t.Errorf("expected %d identities, got %d", calls, repo.creates)
		}
	})

	t.Run("a repeated device call mints a new identity and never reissues the old one", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		deviceUUID := "550e8400-e29b-41d4-a716-446655440000"

		first, err := svc.CreateSession(context.Background(), deviceUUID)
		if err != nil {
			t.Fatalf("first CreateSession returned error: %v", err)
		}
		second, err := svc.CreateSession(context.Background(), deviceUUID)
		if err != nil {
			t.Fatalf("second CreateSession returned error: %v", err)
		}

		if second.AnonymousID == first.AnonymousID {
			t.Errorf("a known device UUID must not reissue identity %q", first.AnonymousID)
		}
		if second.Token == first.Token {
			t.Error("expected distinct tokens for distinct identities")
		}
		// The first session must remain valid: the second call never touched it.
		if id, ok, err := svc.Authenticate(context.Background(), first.Token); err != nil || !ok || id != first.AnonymousID {
			t.Errorf("first token must keep authenticating its identity: id=%q ok=%v err=%v", id, ok, err)
		}
		if repo.creates != 2 {
			t.Errorf("expected two identities, got %d", repo.creates)
		}
	})

	t.Run("a device UUID already claimed yields a fresh identity, not the owner's", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		deviceUUID := "550e8400-e29b-41d4-a716-446655440000"
		owner := &AnonIdentity{
			ID:         "c0ffee00-0000-0000-0000-0000000000ff",
			DeviceUUID: &deviceUUID,
			AnonName:   "Anon Existing",
			TokenHash:  strings.Repeat("f", 64),
		}
		repo.byDeviceUUID[deviceUUID] = owner
		repo.byTokenHash[owner.TokenHash] = owner
		repo.takenNames[owner.AnonName] = true

		session, err := svc.CreateSession(context.Background(), deviceUUID)
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if session.AnonymousID == owner.ID {
			t.Error("knowing a device UUID must not hand back the owner's identity")
		}
		if _, ok := repo.byTokenHash[HashToken(session.Token)]; !ok {
			t.Error("expected the new session's token hash to be stored")
		}
	})

	t.Run("retries when inserts collide with the unique constraints", func(t *testing.T) {
		repo := newFakeRepository()
		repo.conflicts = 2 // force the first two inserts to collide
		svc := NewService(repo)

		session, err := svc.CreateSession(context.Background(), "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}
		if session.AnonymousID == "" {
			t.Error("expected a non-empty anonymous id after retries")
		}
		if _, ok := repo.byTokenHash[HashToken(session.Token)]; !ok {
			t.Error("expected the final token hash to be stored")
		}
	})

	t.Run("gives up after exhausting retries", func(t *testing.T) {
		repo := newFakeRepository()
		repo.conflicts = maxCreateAttempts + 3
		svc := NewService(repo)

		_, err := svc.CreateSession(context.Background(), "")
		if !errors.Is(err, ErrIdentityConflict) {
			t.Fatalf("expected ErrIdentityConflict after exhausting retries, got %v", err)
		}
	})
}

func TestServiceAuthenticate(t *testing.T) {
	t.Run("a valid token repeatedly returns the same identity", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		session, err := svc.CreateSession(context.Background(), "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}

		for i := 0; i < 3; i++ {
			id, ok, err := svc.Authenticate(context.Background(), session.Token)
			if err != nil {
				t.Fatalf("Authenticate returned error: %v", err)
			}
			if !ok {
				t.Fatal("expected the freshly minted token to authenticate")
			}
			if id != session.AnonymousID {
				t.Errorf("expected id %q, got %q", session.AnonymousID, id)
			}
		}
	})

	t.Run("a wrong token never returns another user's session", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		victim, err := svc.CreateSession(context.Background(), "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}

		wrong, err := GenerateRawToken()
		if err != nil {
			t.Fatalf("GenerateRawToken returned error: %v", err)
		}
		id, ok, err := svc.Authenticate(context.Background(), wrong)
		if err != nil {
			t.Fatalf("Authenticate returned error for a wrong token: %v", err)
		}
		if ok || id != "" {
			t.Errorf("a wrong token must not resolve to an identity, got ok=%v id=%q (victim=%q)", ok, id, victim.AnonymousID)
		}
	})

	t.Run("unknown token reports ok=false, not a failure", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		raw, err := GenerateRawToken()
		if err != nil {
			t.Fatalf("GenerateRawToken returned error: %v", err)
		}
		id, ok, err := svc.Authenticate(context.Background(), raw)
		if err != nil {
			t.Fatalf("Authenticate returned error for an unknown token: %v", err)
		}
		if ok || id != "" {
			t.Errorf("unknown token must authenticate as false, got ok=%v id=%q", ok, id)
		}
	})

	t.Run("empty token reports ok=false", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		if _, ok, err := svc.Authenticate(context.Background(), ""); ok || err != nil {
			t.Errorf("expected empty token → ok=false, err=nil, got ok=%v err=%v", ok, err)
		}
		if _, ok, err := svc.Authenticate(context.Background(), "   "); ok || err != nil {
			t.Errorf("expected whitespace token → ok=false, err=nil, got ok=%v err=%v", ok, err)
		}
	})
}
