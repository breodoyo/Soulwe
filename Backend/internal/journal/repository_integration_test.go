//go:build integration

package journal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"Backend/internal/cipher"
	"Backend/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

// integrationKey is a fixed 32-byte key used only by these tests. Production
// keys come from JOURNAL_ENCRYPTION_KEY and are never present in code.
const (
	integrationKey  = "0123456789abcdef0123456789abcdef"
	integrationText = "Integration diary: the rain finally stopped and the town smelled of wet earth."
)

// TestPostgresRepositoryIntegration exercises the journal repository against a
// running PostgreSQL. It is excluded from the default build via the
// "integration" tag and skipped when DATABASE_URL is not set. Two users are
// created so ownership isolation and cascade deletes can be verified.
func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	users := user.NewPostgresRepository(pool)
	repo := NewPostgresRepository(pool)
	codec, err := cipher.NewAESGCM([]byte(integrationKey))
	if err != nil {
		t.Fatalf("codec: %v", err)
	}

	const passwordHash = "$2a$12$abcdefghijklmnopqrstuv" // arbitrary bcrypt-shaped value

	seededUserIDs := make([]string, 0, 3)
	seedUser := func() string {
		u := &user.User{
			Email:        fmt.Sprintf("journal-owner-%d@soulwe.local", time.Now().UnixNano()),
			PasswordHash: passwordHash,
			LanguagePref: "en",
		}
		if err := users.Create(ctx, u); err != nil {
			t.Fatalf("failed to seed user: %v", err)
		}
		seededUserIDs = append(seededUserIDs, u.ID)
		return u.ID
	}

	defer func() {
		for _, id := range seededUserIDs {
			if _, err := pool.Exec(context.Background(),
				"DELETE FROM journal_entries WHERE user_id = $1", id); err != nil {
				t.Logf("journal cleanup for user %s: %v", id, err)
			}
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("cleanup failed to DELETE test user %s: %v", id, err)
			}
		}
	}()

	ownerID := seedUser()
	otherID := seedUser()

	encrypt := func(text string) ([]byte, []byte) {
		t.Helper()
		enc, iv, err := codec.Encrypt([]byte(text), []byte(ownerID))
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		return enc, iv
	}

	newEntry := func(text string, tags []string) *JournalEntry {
		enc, iv := encrypt(text)
		return &JournalEntry{
			UserID:     ownerID,
			ContentEnc: enc,
			ContentIV:  iv,
			MoodTags:   tags,
			PromptUsed: ptr("My day"),
			WordCount:  len(bytes.Fields([]byte(text))),
		}
	}

	t.Run("Create stores ciphertext and returns a UUID entry", func(t *testing.T) {
		entry := newEntry(integrationText, []string{"Heavy", "Loved"})
		if err := repo.Create(ctx, entry); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if len(entry.ID) != 36 {
			t.Errorf("expected a UUID id, got %q", entry.ID)
		}
		if entry.CreatedAt.IsZero() {
			t.Error("expected created_at to be populated")
		}

		// No plaintext may reach the database.
		var storedEnc []byte
		if err := pool.QueryRow(ctx,
			"SELECT content_enc FROM journal_entries WHERE id = $1", entry.ID).Scan(&storedEnc); err != nil {
			t.Fatalf("raw SELECT failed: %v", err)
		}
		if bytes.Contains(storedEnc, []byte("rain")) {
			t.Error("plaintext appears inside the stored ciphertext")
		}

		got, err := repo.GetByID(ctx, ownerID, entry.ID)
		if err != nil {
			t.Fatalf("GetByID returned error: %v", err)
		}
		plain, err := codec.Decrypt(got.ContentEnc, got.ContentIV, []byte(ownerID))
		if err != nil {
			t.Fatalf("decrypt failed: %v", err)
		}
		if string(plain) != integrationText {
			t.Errorf("round trip mismatch: %q", plain)
		}
		if got.WordCount != len(bytes.Fields([]byte(integrationText))) {
			t.Errorf("expected word count %d, got %d", len(bytes.Fields([]byte(integrationText))), got.WordCount)
		}
		if len(got.MoodTags) != 2 || got.MoodTags[0] != "Heavy" || got.MoodTags[1] != "Loved" {
			t.Errorf("unexpected mood tags: %v", got.MoodTags)
		}
		if got.PromptUsed == nil || *got.PromptUsed != "My day" {
			t.Errorf("unexpected prompt_used: %v", got.PromptUsed)
		}
	})

	t.Run("entries are scoped to the owner", func(t *testing.T) {
		entry := newEntry("private thoughts.", nil)
		if err := repo.Create(ctx, entry); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		if _, err := repo.GetByID(ctx, otherID, entry.ID); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("expected ErrJournalEntryNotFound for another user, got %v", err)
		}
		owned, err := repo.GetByID(ctx, ownerID, entry.ID)
		if err != nil {
			t.Fatalf("owner GetByID: %v", err)
		}
		if plain, _ := codec.Decrypt(owned.ContentEnc, owned.ContentIV, []byte(ownerID)); string(plain) != "private thoughts." {
			t.Errorf("unexpected round trip: %q", plain)
		}
		// The same ciphertext cannot be decrypted under the other user's ID.
		if _, err := codec.Decrypt(owned.ContentEnc, owned.ContentIV, []byte(otherID)); err == nil {
			t.Error("ciphertext decrypted under a foreign ownership tag")
		}
	})

	t.Run("ListByUserID returns the user's entries newest first", func(t *testing.T) {
		titles := []string{"first", "second", "third", "fourth", "fifth"}
		for i, title := range titles {
			if err := repo.Create(ctx, newEntry(title+" entry.", []string{"Calm"})); err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			time.Sleep(time.Millisecond)
			if i == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}

		entries, err := repo.ListByUserID(ctx, ownerID, 100, nil)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(entries) < 5 {
			t.Fatalf("expected at least 5 entries, got %d", len(entries))
		}
		for i := 1; i < len(entries); i++ {
			if entries[i-1].CreatedAt.Before(entries[i].CreatedAt) {
				t.Errorf("list not newest-first at index %d: %v before %v",
					i, entries[i-1].CreatedAt, entries[i].CreatedAt)
			}
		}

		limited, err := repo.ListByUserID(ctx, ownerID, 3, nil)
		if err != nil {
			t.Fatalf("ListByUserID(limit) returned error: %v", err)
		}
		if len(limited) != 3 {
			t.Errorf("expected 3 entries, got %d", len(limited))
		}
	})

	t.Run("before cursor resumes from an earlier created_at", func(t *testing.T) {
		entries, err := repo.ListByUserID(ctx, ownerID, 2, nil)
		if err != nil {
			t.Fatalf("ListByUserID returned error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries to page over, got %d", len(entries))
		}
		nextPage, err := repo.ListByUserID(ctx, ownerID, 2, &entries[1].CreatedAt)
		if err != nil {
			t.Fatalf("paged ListByUserID returned error: %v", err)
		}
		if len(nextPage) == 0 {
			t.Fatal("expected a second page of entries")
		}
		for _, e := range nextPage {
			if !e.CreatedAt.Before(entries[1].CreatedAt) {
				t.Errorf("pagination returned an entry not older than the cursor")
			}
		}
	})

	t.Run("Update re-encrypts content and clears the stored reflection", func(t *testing.T) {
		entry := newEntry("original words.", []string{"Old"})
		entry.AIReflection = ptr("stale reflection")
		if err := repo.Create(ctx, entry); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		changed := *newEntry("revised words.", []string{"New"})
		changed.AIReflection = nil
		if err := repo.Update(ctx, ownerID, entry.ID, &changed, true); err != nil {
			t.Fatalf("Update content: %v", err)
		}
		if changed.AIReflection != nil {
			t.Error("expected the reflection to be cleared on a content change")
		}
		if changed.ID != entry.ID {
			t.Error("UPDATE RETURNING must preserve the row id")
		}
		plain, err := codec.Decrypt(changed.ContentEnc, changed.ContentIV, []byte(ownerID))
		if err != nil {
			t.Fatalf("decrypt after update: %v", err)
		}
		if string(plain) != "revised words." {
			t.Errorf("expected the revised text to be stored, got %q", plain)
		}

		// Metadata-only change: content and any stored reflection survive.
		if err := repo.UpdateReflection(ctx, ownerID, entry.ID, "keep me"); err != nil {
			t.Fatalf("UpdateReflection: %v", err)
		}
		meta := changed
		meta.MoodTags = []string{"Calm"}
		if err := repo.Update(ctx, ownerID, entry.ID, &meta, false); err != nil {
			t.Fatalf("Update metadata: %v", err)
		}
		if meta.AIReflection == nil || *meta.AIReflection != "keep me" {
			t.Errorf("expected the reflection to survive a metadata-only update, got %v", meta.AIReflection)
		}
		plainAfter, _ := codec.Decrypt(meta.ContentEnc, meta.ContentIV, []byte(ownerID))
		if string(plainAfter) != "revised words." {
			t.Errorf("content must be untouched by a metadata-only update, got %q", plainAfter)
		}

		if err := repo.Update(ctx, otherID, entry.ID, &changed, true); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("expected ErrJournalEntryNotFound updating another user's entry, got %v", err)
		}
		if err := repo.UpdateReflection(ctx, otherID, entry.ID, "theft"); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("expected ErrJournalEntryNotFound reflecting another user's entry, got %v", err)
		}
	})

	t.Run("Delete removes the entry and is scoped to the owner", func(t *testing.T) {
		entry := newEntry("to be removed.", nil)
		if err := repo.Create(ctx, entry); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		if err := repo.Delete(ctx, otherID, entry.ID); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("expected ErrJournalEntryNotFound deleting another user's entry, got %v", err)
		}
		if err := repo.Delete(ctx, ownerID, entry.ID); err != nil {
			t.Fatalf("owner Delete: %v", err)
		}
		if _, err := repo.GetByID(ctx, ownerID, entry.ID); !errors.Is(err, ErrJournalEntryNotFound) {
			t.Errorf("expected the deleted entry to be gone, got %v", err)
		}
	})

	t.Run("deleting the user cascades their journal entries", func(t *testing.T) {
		ghostID := seedUser()
		ghostCodec, err := cipher.NewAESGCM([]byte(integrationKey))
		if err != nil {
			t.Fatalf("ghost codec: %v", err)
		}
		for i := 0; i < 3; i++ {
			enc, iv, err := ghostCodec.Encrypt([]byte(fmt.Sprintf("ghost %d", i)), []byte(ghostID))
			if err != nil {
				t.Fatalf("ghost encrypt: %v", err)
			}
			ghost := &JournalEntry{UserID: ghostID, ContentEnc: enc, ContentIV: iv, MoodTags: []string{}}
			if err := repo.Create(ctx, ghost); err != nil {
				t.Fatalf("ghost Create: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", ghostID); err != nil {
			t.Fatalf("failed to DELETE the ghost user: %v", err)
		}

		ghostEntries, err := repo.ListByUserID(ctx, ghostID, 100, nil)
		if err != nil {
			t.Fatalf("ListByUserID for ghost: %v", err)
		}
		if len(ghostEntries) != 0 {
			t.Errorf("expected the ghost's entries to cascade-delete, got %d", len(ghostEntries))
		}
	})
}
