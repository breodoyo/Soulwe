package anon

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepository is an in-memory Repository used to unit-test the service
// without a real PostgreSQL connection. token_hash values are stored as
// provided (already hashed), and anon_name uniqueness mimics the UNIQUE
// constraint on the real table.
type fakeRepository struct {
	byTokenHash  map[string]*AnonIdentity
	byDeviceUUID map[string]*AnonIdentity
	takenNames   map[string]bool
	nextID       int
	lastSeenAt   map[string]bool
	rotations    int
	creates      int
	conflicts    int // number of Create calls that should collide, to test retries

	// deviceCreatedRacing simulates a concurrent request that created this
	// device's identity between our initial FindByDeviceUUID and this insert.
	// The first Create for a device fires once and leaves the row behind.
	deviceCreatedRacing bool
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
	if f.conflicts > 0 {
		f.conflicts--
		return ErrIdentityConflict
	}
	if f.deviceCreatedRacing && identity.DeviceUUID != nil {
		f.deviceCreatedRacing = false // fire once
		existing := &AnonIdentity{
			ID:         "c0ffee00-0000-0000-0000-0000000000ff",
			DeviceUUID: identity.DeviceUUID,
			AnonName:   "Anon Existing",
			TokenHash:  strings.Repeat("f", 64),
		}
		f.takenNames[existing.AnonName] = true
		f.byTokenHash[existing.TokenHash] = existing
		f.byDeviceUUID[*identity.DeviceUUID] = existing
		return ErrIdentityConflict
	}
	if f.byTokenHash[identity.TokenHash] != nil {
		return ErrIdentityConflict
	}
	if f.takenNames[identity.AnonName] {
		return ErrIdentityConflict
	}
	f.nextID++
	identity.ID = string(rune('a'+f.nextID)) + strings.Repeat("0", 35)
	f.takenNames[identity.AnonName] = true
	f.byTokenHash[identity.TokenHash] = identity
	if identity.DeviceUUID != nil {
		if existing := f.byDeviceUUID[*identity.DeviceUUID]; existing != nil {
			return ErrIdentityConflict
		}
		f.byDeviceUUID[*identity.DeviceUUID] = identity
	}
	f.creates++
	return nil
}

func (f *fakeRepository) FindByTokenHash(_ context.Context, tokenHash string) (*AnonIdentity, error) {
	identity, ok := f.byTokenHash[tokenHash]
	if !ok {
		return nil, ErrIdentityNotFound
	}
	return identity, nil
}

func (f *fakeRepository) FindByDeviceUUID(_ context.Context, deviceUUID string) (*AnonIdentity, error) {
	identity, ok := f.byDeviceUUID[deviceUUID]
	if !ok {
		return nil, ErrIdentityNotFound
	}
	return identity, nil
}

func (f *fakeRepository) UpdateLastSeen(_ context.Context, id string) error {
	f.lastSeenAt[id] = true
	return nil
}

func (f *fakeRepository) RotateToken(_ context.Context, id, tokenHash string) error {
	identity := f.byID(id)
	if identity == nil {
		return ErrIdentityNotFound
	}
	delete(f.byTokenHash, identity.TokenHash)
	identity.TokenHash = tokenHash
	f.byTokenHash[tokenHash] = identity
	f.lastSeenAt[id] = true
	f.rotations++
	return nil
}

func (f *fakeRepository) byID(id string) *AnonIdentity {
	for _, identity := range f.byTokenHash {
		if identity.ID == id {
			return identity
		}
	}
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
		// The raw token must never be persisted; the hash must be.
		if _, ok := repo.byTokenHash[session.Token]; ok {
			t.Error("the raw token was stored as the token_hash — only the hash should be stored")
		}
		if _, ok := repo.byTokenHash[HashToken(session.Token)]; !ok {
			t.Error("expected the SHA-256 hash of the token to be stored")
		}
	})

	t.Run("same device is idempotent and keeps the same anonymous id", func(t *testing.T) {
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

		if second.AnonymousID != first.AnonymousID {
			t.Errorf("same device must keep anonymous_id %q, got %q", first.AnonymousID, second.AnonymousID)
		}
		if second.Token == first.Token {
			t.Error("the token should rotate on a repeated device call")
		}
		if repo.creates != 1 {
			t.Errorf("same device must not create a second identity, creates=%d", repo.creates)
		}
		if repo.rotations != 1 {
			t.Errorf("expected one token rotation, got %d", repo.rotations)
		}
		// Old token must no longer authenticate after the rotation.
		if _, ok := repo.byTokenHash[HashToken(first.Token)]; ok {
			t.Error("old token hash should be replaced after rotation")
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

	t.Run("loses a concurrent create race but reuses the winner's identity", func(t *testing.T) {
		// Simulate two requests starting with the same new device UUID: our
		// initial lookup misses, another request inserts first, and our insert
		// collides on the device_uuid unique index on every retry. The service
		// must re-check and rotate the winner instead of returning a 500.
		repo := newFakeRepository()
		repo.deviceCreatedRacing = true
		svc := NewService(repo)
		deviceUUID := "550e8400-e29b-41d4-a716-446655440000"

		session, err := svc.CreateSession(context.Background(), deviceUUID)
		if err != nil {
			t.Fatalf("CreateSession returned error instead of reusing the race winner: %v", err)
		}
		if session.AnonymousID != "c0ffee00-0000-0000-0000-0000000000ff" {
			t.Errorf("expected the concurrent winner's anonymous id, got %q", session.AnonymousID)
		}
		if repo.creates != 0 {
			t.Errorf("expected no successful insert from the losing request, creates=%d", repo.creates)
		}
		if repo.rotations != 1 {
			t.Errorf("expected the winner's token to be rotated once, rotations=%d", repo.rotations)
		}
		if session.Token == "" {
			t.Error("expected a fresh token to rotate the winner's identity")
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
	t.Run("valid token returns the identity id", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		session, err := svc.CreateSession(context.Background(), "")
		if err != nil {
			t.Fatalf("CreateSession returned error: %v", err)
		}

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
