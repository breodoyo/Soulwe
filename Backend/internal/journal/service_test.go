package journal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"Backend/internal/cipher"
	"Backend/internal/middleware"
)

const (
	testKey    = "0123456789abcdef0123456789abcdef"
	secretText = "Today was exhausting but holding my daughter helped."
)

func ptr[T any](v T) *T { return &v }

func testCodec(t *testing.T) *cipher.AESGCM {
	t.Helper()
	c, err := cipher.NewAESGCM([]byte(testKey))
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	return c
}

type fakeReflection struct {
	out    string
	err    error
	calls  int
	got    string
	gotTag []string
}

func (f *fakeReflection) GenerateReflection(ctx context.Context, content string, tags []string) (string, error) {
	f.calls++
	f.got = content
	f.gotTag = tags
	if f.err != nil {
		return "", f.err
	}
	if f.out == "" {
		return "", fmt.Errorf("fake reflection produced nothing")
	}
	return f.out, nil
}

func owner(userID string) middleware.Owner { return middleware.Owner{UserID: userID} }

func anon(anonID string) middleware.Owner { return middleware.Owner{AnonIdentityID: anonID} }

// ownerKey namespaces owners so a user and an anonymous session can never collide
// in the fake repository's index, mirroring the schema's one-owner-per-row rule.
func ownerKey(o middleware.Owner) string {
	if o.Registered() {
		return "user:" + o.UserID
	}
	return "anon:" + o.AnonIdentityID
}

// ownerKeyOf reads the owner columns off a stored entry.
func ownerKeyOf(e *JournalEntry) string {
	return ownerKey(middleware.Owner{UserID: e.UserID, AnonIdentityID: e.AnonIdentityID})
}

// ownedBy reports whether a stored entry belongs to the given owner.
func ownedBy(e *JournalEntry, o middleware.Owner) bool {
	return ownerKeyOf(e) == ownerKey(o)
}

// fakeRepository is an in-memory Repository that stores encrypted content
// verbatim, like PostgreSQL, so tests can assert plaintext never reaches it.
type fakeRepository struct {
	mu      sync.Mutex
	entries map[string]*JournalEntry // by id
	order   map[string][]string      // owner key -> ids, newest first
	seq     int
	now     time.Time
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		entries: map[string]*JournalEntry{},
		order:   map[string][]string{},
	}
}

func (f *fakeRepository) Create(ctx context.Context, e *JournalEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	e.ID = fmt.Sprintf("entry-%d", f.seq)
	e.CreatedAt = f.now.Add(time.Duration(f.seq) * time.Second)
	f.entries[e.ID] = cloneEntry(e)
	key := ownerKeyOf(e)
	f.order[key] = append(f.order[key], e.ID)
	return nil
}

func (f *fakeRepository) ListByOwner(ctx context.Context, owner middleware.Owner, limit int, before *time.Time) ([]JournalEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]JournalEntry, 0)
	ids := f.order[ownerKey(owner)]
	for i := len(ids) - 1; i >= 0; i-- {
		e := f.entries[ids[i]]
		if before != nil && !e.CreatedAt.Before(*before) {
			continue
		}
		out = append(out, *cloneEntry(e))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepository) GetByID(ctx context.Context, owner middleware.Owner, entryID string) (*JournalEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[entryID]
	if !ok || !ownedBy(e, owner) {
		return nil, ErrJournalEntryNotFound
	}
	return cloneEntry(e), nil
}

func (f *fakeRepository) Update(ctx context.Context, owner middleware.Owner, entryID string, e *JournalEntry, contentChanged bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.entries[entryID]
	if !ok || !ownedBy(stored, owner) {
		return ErrJournalEntryNotFound
	}
	merged := *stored
	merged.MoodTags = append([]string(nil), e.MoodTags...)
	merged.PromptUsed = e.PromptUsed
	if contentChanged {
		merged.ContentEnc = e.ContentEnc
		merged.ContentIV = e.ContentIV
		merged.WordCount = e.WordCount
		merged.AIReflection = nil
	}
	f.entries[entryID] = &merged
	*e = *cloneEntry(f.entries[entryID])
	return nil
}

func (f *fakeRepository) UpdateReflection(ctx context.Context, owner middleware.Owner, entryID, reflection string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.entries[entryID]
	if !ok || !ownedBy(stored, owner) {
		return ErrJournalEntryNotFound
	}
	stored.AIReflection = ptr(reflection)
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, owner middleware.Owner, entryID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[entryID]
	if !ok || !ownedBy(e, owner) {
		return ErrJournalEntryNotFound
	}
	delete(f.entries, entryID)
	ids := f.order[ownerKey(owner)]
	for i, id := range ids {
		if id == entryID {
			key := ownerKey(owner)
			f.order[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeRepository) stored(owner middleware.Owner, entryID string) *JournalEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.entries[entryID]
	if e == nil || !ownedBy(e, owner) {
		return nil
	}
	return cloneEntry(e)
}

func cloneEntry(e *JournalEntry) *JournalEntry {
	c := *e
	c.ContentEnc = append([]byte(nil), e.ContentEnc...)
	c.ContentIV = append([]byte(nil), e.ContentIV...)
	c.MoodTags = append([]string(nil), e.MoodTags...)
	return &c
}

func testCodecBytes(aadOwner middleware.Owner, content string) ([]byte, []byte, error) {
	codec, err := cipher.NewAESGCM([]byte(testKey))
	if err != nil {
		return nil, nil, err
	}
	aad, err := encryptionAAD(aadOwner)
	if err != nil {
		return nil, nil, err
	}
	enc, iv, err := codec.Encrypt([]byte(content), aad)
	return enc, iv, err
}

func TestCreateEncryptsBeforeStoring(t *testing.T) {
	repo := newFakeRepository()
	ref := &fakeReflection{out: "Carrying that weight yet still showing up is a sign of strength."}
	svc := NewService(repo, testCodec(t), ref)

	entry, err := svc.Create(context.Background(), owner("user-1"),
		"  "+secretText+"  ", []string{"Tired", "Tired", "Loved"}, ptr("My day"))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if entry.ID == "" {
		t.Error("expected a generated id")
	}
	if entry.WordCount != 8 {
		t.Errorf("expected word count 8, got %d", entry.WordCount)
	}
	if len(entry.MoodTags) != 2 || entry.MoodTags[0] != "Tired" || entry.MoodTags[1] != "Loved" {
		t.Errorf("expected deduplicated trimmed tags, got %v", entry.MoodTags)
	}
	if entry.PromptUsed == nil || *entry.PromptUsed != "My day" {
		t.Errorf("expected prompt_used 'My day', got %v", entry.PromptUsed)
	}
	if entry.Content != "" {
		t.Error("create response must not carry plaintext content")
	}
	if ref.calls != 1 {
		t.Errorf("expected reflection generated during create, got %d calls", ref.calls)
	}
	if entry.AIReflection == nil {
		t.Error("expected a stored reflection")
	}

	stored := repo.stored(owner("user-1"), entry.ID)
	if stored == nil {
		t.Fatal("entry was not persisted")
	}
	if bytes.Contains(stored.ContentEnc, []byte(secretText)) {
		t.Error("ciphertext must not contain the plaintext")
	}
	if bytes.Equal(stored.ContentEnc, []byte(secretText)) {
		t.Error("plaintext was stored without encryption")
	}
	if len(stored.ContentIV) != cipher.NonceSize {
		t.Errorf("expected a %d-byte IV, got %d", cipher.NonceSize, len(stored.ContentIV))
	}

	plain, err := testCodec(t).Decrypt(stored.ContentEnc, stored.ContentIV, []byte("user-1"))
	if err != nil {
		t.Fatalf("stored ciphertext could not be decrypted: %v", err)
	}
	if string(plain) != secretText {
		t.Errorf("round trip mismatch: %q", plain)
	}

	// A different user ID (ownership tag) must not decrypt.
	if _, err := testCodec(t).Decrypt(stored.ContentEnc, stored.ContentIV, []byte("user-2")); err == nil {
		t.Error("ciphertext must not decrypt under another user's ownership tag")
	}
}

func TestCreateContentValidation(t *testing.T) {
	codec := testCodec(t)
	long := strings.Repeat("a", MaxContentLength+1)

	for name, tc := range map[string]struct {
		content string
		want    error
	}{
		"empty":                {"", ErrInvalidContent},
		"whitespace only":      {"   \n\t ", ErrInvalidContent},
		"over max length":      {long, ErrInvalidContent},
		"exactly max is valid": {strings.Repeat("a", MaxContentLength), nil},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := NewService(repo, codec, &fakeReflection{out: "ok"})
			entry, err := svc.Create(context.Background(), owner("user-1"), tc.content, nil, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
			if tc.want == nil && entry == nil {
				t.Fatal("expected a created entry")
			}
			if tc.want != nil && len(repo.order["user-1"]) != 0 {
				t.Error("invalid content must not be persisted")
			}
		})
	}
}

func TestCreateMoodTagValidation(t *testing.T) {
	codec := testCodec(t)
	tooMany := make([]string, MaxMoodTags+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("tag-%d", i)
	}

	for name, tc := range map[string]struct {
		tags []string
		want error
	}{
		"too many unique tags":     {tooMany, ErrInvalidMoodTags},
		"one tag too long":         {[]string{strings.Repeat("x", MaxMoodTagLength+1)}, ErrInvalidMoodTags},
		"empty tags are valid":     {[]string{}, nil},
		"blank tags are dropped":   {[]string{"  ", "Loved"}, nil},
		"under the cap is valid":   {[]string{"A", "B"}, nil},
		"over max length boundary": {[]string{strings.Repeat("x", MaxMoodTagLength)}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository()
			svc := NewService(repo, codec, &fakeReflection{out: "ok"})
			_, err := svc.Create(context.Background(), owner("user-1"), secretText, tc.tags, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
			if tc.want != nil && len(repo.order["user-1"]) != 0 {
				t.Error("invalid mood tags must not be persisted")
			}
		})
	}
}

func TestCreatePromptValidation(t *testing.T) {
	codec := testCodec(t)
	tooLong := strings.Repeat("p", MaxPromptLength+1)

	t.Run("prompt too long is rejected", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, nil)
		_, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, ptr(tooLong))
		if !errors.Is(err, ErrInvalidPromptUsed) {
			t.Fatalf("expected ErrInvalidPromptUsed, got %v", err)
		}
	})

	t.Run("blank prompt is stored as null", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, nil)
		entry, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, ptr("   "))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if entry.PromptUsed != nil {
			t.Errorf("expected a nil prompt_used, got %v", *entry.PromptUsed)
		}
	})

	t.Run("prompt is trimmed", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, nil)
		entry, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, ptr("  My day "))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if entry.PromptUsed == nil || *entry.PromptUsed != "My day" {
			t.Errorf("expected trimmed prompt, got %v", entry.PromptUsed)
		}
	})
}

func TestCreateReflectionIsBestEffort(t *testing.T) {
	codec := testCodec(t)

	t.Run("no generator configured", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, nil)
		entry, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		if err != nil {
			t.Fatalf("Create must succeed without a generator: %v", err)
		}
		if entry.AIReflection != nil {
			t.Error("expected nil reflection when no generator is configured")
		}
	})

	t.Run("generator failure still saves the entry", func(t *testing.T) {
		repo := newFakeRepository()
		ref := &fakeReflection{err: errors.New("upstream is down")}
		svc := NewService(repo, codec, ref)
		entry, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		if err != nil {
			t.Fatalf("Create must not fail when reflection fails: %v", err)
		}
		if entry.AIReflection != nil {
			t.Error("expected nil reflection after a generator failure")
		}
		if repo.stored(owner("user-1"), entry.ID) == nil {
			t.Error("entry must still be persisted when reflection fails")
		}
	})

	t.Run("only the entry text is handed to the generator", func(t *testing.T) {
		repo := newFakeRepository()
		ref := &fakeReflection{out: "ok"}
		svc := NewService(repo, codec, ref)
		_, err := svc.Create(context.Background(), owner("user-1"), secretText, []string{"Calm"}, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if ref.got != secretText {
			t.Errorf("generator received %q, want the exact trimmed content", ref.got)
		}
		if len(ref.gotTag) != 1 || ref.gotTag[0] != "Calm" {
			t.Errorf("generator received unexpected tags: %v", ref.gotTag)
		}
	})
}

func TestListScopedAndClamped(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, testCodec(t), nil)

	for i := 0; i < 60; i++ {
		_, err := svc.Create(context.Background(), owner("user-1"), fmt.Sprintf("entry number %d", i), nil, nil)
		if err != nil {
			t.Fatalf("seed user-1: %v", err)
		}
	}
	_, err := svc.Create(context.Background(), owner("user-2"), "someone else's diary", nil, nil)
	if err != nil {
		t.Fatalf("seed user-2: %v", err)
	}

	t.Run("clamps to the requested limit", func(t *testing.T) {
		entries, err := svc.List(context.Background(), owner("user-1"), 5, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("expected 5 entries, got %d", len(entries))
		}
		for _, e := range entries {
			if e.Content != "" {
				t.Error("list responses must never include content")
			}
			if e.UserID != "" {
				t.Error("list responses must never include user_id")
			}
		}
	})

	t.Run("applies the default limit and caps the maximum", func(t *testing.T) {
		def, err := svc.List(context.Background(), owner("user-1"), 0, nil)
		if err != nil {
			t.Fatalf("List default: %v", err)
		}
		if len(def) != DefaultListLimit {
			t.Errorf("expected default %d entries, got %d", DefaultListLimit, len(def))
		}
		capped, err := svc.List(context.Background(), owner("user-1"), 999, nil)
		if err != nil {
			t.Fatalf("List capped: %v", err)
		}
		if len(capped) != MaxListLimit {
			t.Errorf("expected cap %d entries, got %d", MaxListLimit, len(capped))
		}
	})

	t.Run("returns newest first", func(t *testing.T) {
		entries, err := svc.List(context.Background(), owner("user-1"), 3, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if entries[0].CreatedAt.Before(entries[2].CreatedAt) {
			t.Errorf("expected newest-first ordering, got %v then %v", entries[0].CreatedAt, entries[2].CreatedAt)
		}
	})

	t.Run("never leaks another user's entries", func(t *testing.T) {
		entries, err := svc.List(context.Background(), owner("user-2"), 0, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected exactly the second user's entry, got %d", len(entries))
		}
	})
}

func TestGetDecryptsOwnEntry(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, testCodec(t), &fakeReflection{out: "ok"})

	created, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(context.Background(), owner("user-1"), created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != secretText {
		t.Errorf("expected decrypted content, got %q", got.Content)
	}

	if _, err := svc.Get(context.Background(), owner("user-2"), created.ID); !errors.Is(err, ErrJournalEntryNotFound) {
		t.Errorf("expected ErrJournalEntryNotFound for another user, got %v", err)
	}
}

// An anonymous session is a full owner: its entries must stay as private as a user's.
func TestAnonymousSessionOwnership(t *testing.T) {
	t.Run("encrypts under the anonymous owner's AAD and round-trips", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, testCodec(t), nil)
		session := anon("anon-1")

		created, err := svc.Create(context.Background(), session, secretText, []string{"Tired"}, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.UserID != "" {
			t.Errorf("an anonymous entry must not carry a user id, got %q", created.UserID)
		}

		stored := repo.stored(session, created.ID)
		if stored == nil {
			t.Fatal("anonymous entry was not persisted")
		}
		if stored.AnonIdentityID != "anon-1" {
			t.Errorf("expected the anonymous identity, got %q", stored.AnonIdentityID)
		}
		if bytes.Contains(stored.ContentEnc, []byte(secretText)) {
			t.Error("ciphertext must not contain the plaintext")
		}

		got, err := svc.Get(context.Background(), session, created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Content != secretText {
			t.Errorf("expected decrypted content, got %q", got.Content)
		}
	})

	t.Run("namespaces the AAD so a user cannot decrypt an anonymous entry", func(t *testing.T) {
		repo := newFakeRepository()
		codec := testCodec(t)
		svc := NewService(repo, codec, nil)
		session := anon("anon-1")

		created, err := svc.Create(context.Background(), session, secretText, nil, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		stored := repo.stored(session, created.ID)

		// A user whose ID equals the anonymous identity must not decrypt, which the "anon:" AAD prefix guarantees.
		if _, err := codec.Decrypt(stored.ContentEnc, stored.ContentIV, []byte("anon-1")); err == nil {
			t.Error("a bare-ID AAD decrypted an anonymous entry; the anon namespace is not applied")
		}
	})

	t.Run("isolates two anonymous sessions and a user from each other", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, testCodec(t), nil)
		sessionA, sessionB := anon("anon-1"), anon("anon-2")

		if _, err := svc.Create(context.Background(), sessionA, "A's private words", nil, nil); err != nil {
			t.Fatalf("Create for anon-1: %v", err)
		}
		if _, err := svc.Create(context.Background(), sessionB, "B's private words", nil, nil); err != nil {
			t.Fatalf("Create for anon-2: %v", err)
		}
		if _, err := svc.Create(context.Background(), owner("user-1"), "a user's words", nil, nil); err != nil {
			t.Fatalf("Create for user-1: %v", err)
		}

		aList, err := svc.List(context.Background(), sessionA, 0, nil)
		if err != nil {
			t.Fatalf("List for anon-1: %v", err)
		}
		if len(aList) != 1 {
			t.Fatalf("expected only anon-1's entry, got %d", len(aList))
		}
		// List responses strip ownership, so identify the entry by its ID instead.
		if stored := repo.stored(sessionA, aList[0].ID); stored == nil {
			t.Errorf("list returned an entry anon-1 does not own: %+v", aList[0])
		}

		if _, err := svc.Get(context.Background(), sessionB, aList[0].ID); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("another anonymous session read anon-1's entry: %v", err)
		}
		if _, err := svc.Get(context.Background(), owner("user-1"), aList[0].ID); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("a user read an anonymous session's entry: %v", err)
		}
	})

	t.Run("strip owner columns from list responses", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, testCodec(t), nil)
		session := anon("anon-1")

		if _, err := svc.Create(context.Background(), session, secretText, nil, nil); err != nil {
			t.Fatalf("Create: %v", err)
		}
		entries, err := svc.List(context.Background(), session, 0, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if entries[0].AnonIdentityID != "" || entries[0].UserID != "" {
			t.Errorf("list responses must not carry ownership: %+v", entries[0])
		}
		if entries[0].Content != "" {
			t.Error("list responses must not carry plaintext")
		}
	})
}

func TestUpdate(t *testing.T) {
	codec := testCodec(t)
	repo := newFakeRepository()
	svc := NewService(repo, codec, &fakeReflection{out: "stored reflection"})

	created, err := svc.Create(context.Background(), owner("user-1"), secretText, []string{"Tired"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("metadata change preserves content and reflection", func(t *testing.T) {
		updated, err := svc.Update(context.Background(), owner("user-1"), created.ID, Update{
			MoodTags: ptr([]string{"Hopeful"}),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Content != secretText {
			t.Error("content must be preserved and returned decrypted")
		}
		if updated.AIReflection == nil || *updated.AIReflection != "stored reflection" {
			t.Error("reflection must survive a metadata-only update")
		}
		stored := repo.stored(owner("user-1"), created.ID)
		plain, _ := codec.Decrypt(stored.ContentEnc, stored.ContentIV, []byte("user-1"))
		if string(plain) != secretText {
			t.Error("ciphertext must be untouched by a metadata-only update")
		}
	})

	t.Run("content change re-encrypts and clears the reflection", func(t *testing.T) {
		newText := "The sun came up and so did I."
		updated, err := svc.Update(context.Background(), owner("user-1"), created.ID, Update{
			Content: ptr(newText),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Content != newText {
			t.Errorf("expected new content, got %q", updated.Content)
		}
		if updated.AIReflection != nil {
			t.Error("a stale reflection must be cleared when content changes")
		}
		if updated.WordCount != len(strings.Fields(newText)) {
			t.Errorf("expected updated word count, got %d", updated.WordCount)
		}
		stored := repo.stored(owner("user-1"), created.ID)
		plain, _ := codec.Decrypt(stored.ContentEnc, stored.ContentIV, []byte("user-1"))
		if string(plain) != newText {
			t.Error("stored ciphertext must reflect the new content")
		}
	})

	t.Run("identical content preserves the reflection", func(t *testing.T) {
		created2, err := svc.Create(context.Background(), owner("user-1"), "untouched words here", []string{"Calm"}, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		svc.entries.UpdateReflection(context.Background(), owner("user-1"), created2.ID, "kept")
		updated, err := svc.Update(context.Background(), owner("user-1"), created2.ID, Update{
			Content: ptr("  untouched words here  "),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.AIReflection == nil || *updated.AIReflection != "kept" {
			t.Error("re-saving identical text must not clear the reflection")
		}
	})

	t.Run("nothing to update returns ErrNothingToUpdate", func(t *testing.T) {
		_, err := svc.Update(context.Background(), owner("user-1"), created.ID, Update{})
		if !errors.Is(err, ErrNothingToUpdate) {
			t.Fatalf("expected ErrNothingToUpdate, got %v", err)
		}
	})

	t.Run("invalid content is rejected", func(t *testing.T) {
		_, err := svc.Update(context.Background(), owner("user-1"), created.ID, Update{Content: ptr("  ")})
		if !errors.Is(err, ErrInvalidContent) {
			t.Fatalf("expected ErrInvalidContent, got %v", err)
		}
	})

	t.Run("update is scoped to the owner", func(t *testing.T) {
		_, err := svc.Update(context.Background(), owner("user-2"), created.ID, Update{MoodTags: ptr([]string{"X"})})
		if !errors.Is(err, ErrJournalEntryNotFound) {
			t.Fatalf("expected ErrJournalEntryNotFound, got %v", err)
		}
	})
}

func TestDeleteScopedToOwner(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, testCodec(t), nil)

	created, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(context.Background(), owner("user-2"), created.ID); !errors.Is(err, ErrJournalEntryNotFound) {
		t.Fatalf("expected ErrJournalEntryNotFound for another user, got %v", err)
	}
	if repo.stored(owner("user-1"), created.ID) == nil {
		t.Fatal("entry must survive a foreign delete attempt")
	}

	if err := svc.Delete(context.Background(), owner("user-1"), created.ID); err != nil {
		t.Fatalf("owner delete failed: %v", err)
	}
	if repo.stored(owner("user-1"), created.ID) != nil {
		t.Error("owner's entry must be removed")
	}
}

func TestReflect(t *testing.T) {
	codec := testCodec(t)

	t.Run("generates and persists a reflection", func(t *testing.T) {
		repo := newFakeRepository()
		ref := &fakeReflection{out: "A tender thought."}
		svc := NewService(repo, codec, ref)

		created, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		entry, err := svc.Reflect(context.Background(), owner("user-1"), created.ID)
		if err != nil {
			t.Fatalf("Reflect: %v", err)
		}
		if entry.AIReflection == nil || *entry.AIReflection != "A tender thought." {
			t.Errorf("expected the generated reflection, got %v", entry.AIReflection)
		}
		if entry.Content != secretText {
			t.Error("reflect returns the owner's decrypted content")
		}
		if ref.got != secretText {
			t.Errorf("generator received %q, want the exact content", ref.got)
		}
		stored := repo.stored(owner("user-1"), created.ID)
		if stored.AIReflection == nil || *stored.AIReflection != "A tender thought." {
			t.Error("reflection must be persisted")
		}
	})

	t.Run("no generator configured", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, nil)
		created, _ := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		_, err := svc.Reflect(context.Background(), owner("user-1"), created.ID)
		if !errors.Is(err, ErrAIReflectionUnavailable) {
			t.Fatalf("expected ErrAIReflectionUnavailable, got %v", err)
		}
	})

	t.Run("generator failure is surfaced safely", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, &fakeReflection{err: errors.New("anthropic exploded")})
		created, _ := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		_, err := svc.Reflect(context.Background(), owner("user-1"), created.ID)
		if !errors.Is(err, ErrAIReflectionUnavailable) {
			t.Fatalf("expected ErrAIReflectionUnavailable, got %v", err)
		}
		if strings.Contains(err.Error(), secretText) {
			t.Error("journal content must never appear in reflection errors")
		}
		if strings.Contains(err.Error(), "anthropic exploded") {
			t.Error("upstream details must never leak from reflection errors")
		}
	})

	t.Run("scoped to the owner", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo, codec, &fakeReflection{out: "ok"})
		created, _ := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
		_, err := svc.Reflect(context.Background(), owner("user-2"), created.ID)
		if !errors.Is(err, ErrJournalEntryNotFound) {
			t.Fatalf("expected ErrJournalEntryNotFound, got %v", err)
		}
	})
}

func TestPlaintextNeverLeaksOutOfService(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, testCodec(t), &fakeReflection{err: errors.New("boom")})

	entry, err := svc.Create(context.Background(), owner("user-1"), secretText, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if entry.Content != "" {
		t.Error("create response must not include content")
	}
	if len(entry.ContentEnc) != 0 || len(entry.ContentIV) != 0 {
		t.Error("create response must not include ciphertext")
	}
	if entry.UserID != "" {
		t.Error("create response must not include user_id")
	}

	entries, err := svc.List(context.Background(), owner("user-1"), 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if entries[0].Content != "" || len(entries[0].ContentEnc) != 0 {
		t.Error("list response must not include content or ciphertext")
	}

	if _, err := svc.Reflect(context.Background(), owner("user-1"), entry.ID); err != nil {
		if strings.Contains(err.Error(), secretText) {
			t.Error("reflect error must not contain journal content")
		}
	}
}
