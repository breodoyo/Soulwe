import { useEffect, useRef, useState, type FormEvent } from 'react'
import { api } from '@/lib/api'
import { isApiError, type JournalEntry } from '@/types'
import styles from './JournalPage.module.css'

type JournalMode = 'journal' | 'prayer'

const PRAYER_PROMPT = 'Prayer'

const moodTags: string[] = [
  'Anxious', 'Hopeful', 'Overwhelmed', 'Grateful',
  'Lonely', 'Calm', 'Proud', 'Grieving',
]

const prompts = [
  { label: 'My day',            placeholder: "What's on your mind today..."                          },
  { label: 'Gratitude',         placeholder: 'Something I am grateful for is...'                     },
  { label: 'A challenge',       placeholder: 'A challenge I am facing is... and I feel...'           },
  { label: 'Let go',            placeholder: 'Something I want to let go of is...'                   },
  { label: 'Family & pressure', placeholder: 'What my family expects of me versus what I feel...'    },
  { label: 'My strength',       placeholder: 'What makes me feel strong as an African is...'         },
]

const blessingVerses = [
  { text: '"The Lord bless you and keep you; the Lord make his face shine on you and be gracious to you."', ref: 'Numbers 6:24-25' },
  { text: '"Trust in the Lord with all your heart and lean not on your own understanding."',                 ref: 'Proverbs 3:5'    },
  { text: '"Be still, and know that I am God."',                                                             ref: 'Psalm 46:10'    },
  { text: '"Cast all your anxiety on him because he cares for you."',                                        ref: '1 Peter 5:7'    },
  { text: '"The Lord is close to the brokenhearted and saves those who are crushed in spirit."',             ref: 'Psalm 34:18'    },
  { text: '"For I know the plans I have for you, declares the Lord, plans to prosper you and not to harm you."', ref: 'Jeremiah 29:11' },
]

function wordsLabel(n: number): string {
  return `${n} ${n === 1 ? 'word' : 'words'}`
}

function formatFullDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString('en-KE', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric',
  })
}

function formatListDate(iso: string): { day: string; month: string } {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return { day: '', month: '' }
  return {
    day: String(d.getDate()),
    month: d.toLocaleDateString('en-KE', { month: 'short' }).replace('.', ''),
  }
}

export default function JournalPage() {
  const [mode, setMode] = useState<JournalMode>('journal')

  // Journal mode state
  const [activePrompt, setActivePrompt] = useState(0)
  const [content, setContent] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])

  // Prayer mode state
  const [gratitude, setGratitude] = useState('')
  const [petition, setPetition] = useState('')
  const [listening, setListening] = useState('')
  const [blessing] = useState(
    () => blessingVerses[Math.floor(Math.random() * blessingVerses.length)]
  )

  // Create flow
  const [saving, setSaving] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createStatus, setCreateStatus] = useState<string | null>(null)
  const [reflection, setReflection] = useState<string | null>(null)
  const [reflectionUnavailable, setReflectionUnavailable] = useState(false)

  // Entry list
  const [entries, setEntries] = useState<JournalEntry[]>([])
  const [listLoading, setListLoading] = useState(true)
  const [listError, setListError] = useState<string | null>(null)
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)
  const [pageNotice, setPageNotice] = useState<string | null>(null)

  // Detail / edit / delete / reflect
  const [detailId, setDetailId] = useState<string | null>(null)
  const [detailEntry, setDetailEntry] = useState<JournalEntry | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [editMode, setEditMode] = useState(false)
  const [editContent, setEditContent] = useState('')
  const [editTags, setEditTags] = useState<string[]>([])
  const [editing, setEditing] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)
  const [reflecting, setReflecting] = useState(false)
  const [reflectError, setReflectError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [detailNotice, setDetailNotice] = useState<string | null>(null)

  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const wordCount = content.trim() === '' ? 0 : content.trim().split(/\s+/).length
  const today = new Date().toLocaleDateString('en-KE', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric',
  })

  useEffect(() => {
    let cancelled = false
    setListLoading(true)
    setListError(null)
    api.journal
      .list({ limit: 20 })
      .then(res => {
        if (cancelled) return
        setEntries(res.entries)
        setNextCursor(res.next_cursor)
        setListLoading(false)
      })
      .catch(err => {
        if (cancelled) return
        setListError(
          isApiError(err)
            ? err.message
            : 'We could not load your journal right now. Please try again.',
        )
        setListLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [tick])

  const switchMode = (m: JournalMode) => {
    setMode(m)
    setReflection(null)
    setReflectionUnavailable(false)
    setCreateError(null)
    setCreateStatus(null)
  }

  const toggleTag = (tag: string) => {
    setSelectedTags(prev =>
      prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    )
  }

  const handleSave = async () => {
    if (saving) return
    const isPrayer = mode === 'prayer'
    const hasContent = isPrayer
      ? gratitude.trim() || petition.trim() || listening.trim()
      : content.trim()
    if (!hasContent) {
      setCreateError('Please write something before saving.')
      return
    }

    setSaving(true)
    setCreateError(null)
    setCreateStatus(null)
    setReflection(null)
    setReflectionUnavailable(false)

    const entryText = isPrayer
      ? `Gratitude: ${gratitude}\nPetition: ${petition}\nListening: ${listening}`
      : content
    const promptUsed = isPrayer ? PRAYER_PROMPT : prompts[activePrompt].label

    try {
      const res = await api.journal.create({
        content: entryText,
        mood_tags: selectedTags,
        prompt_used: promptUsed,
      })
      setEntries(prev => [res.entry, ...prev])
      setContent('')
      setSelectedTags([])
      setGratitude('')
      setPetition('')
      setListening('')
      setCreateStatus(isPrayer ? 'Prayer saved.' : 'Entry saved.')
      if (res.entry.ai_reflection) {
        setReflection(res.entry.ai_reflection)
      } else {
        setReflectionUnavailable(true)
      }
    } catch (err) {
      setCreateError(
        isApiError(err)
          ? err.message
          : 'We could not save your entry right now. Please try again.',
      )
    } finally {
      setSaving(false)
    }
  }

  const openEntry = async (id: string) => {
    setDetailId(id)
    setDetailEntry(null)
    setEditMode(false)
    setEditError(null)
    setDeleteConfirm(false)
    setDeleteError(null)
    setDetailNotice(null)
    setReflectError(null)
    setDetailLoading(true)
    setDetailError(null)
    try {
      const res = await api.journal.get(id)
      setDetailEntry(res.entry)
    } catch (err) {
      setDetailError(
        isApiError(err)
          ? err.message
          : 'We could not open this entry right now. Please try again.',
      )
    } finally {
      setDetailLoading(false)
    }
  }

  const closeDetail = () => {
    setDetailId(null)
    setDetailEntry(null)
    setEditMode(false)
    setEditError(null)
    setDeleteConfirm(false)
    setDeleteError(null)
    setDetailNotice(null)
  }

  const startEdit = () => {
    if (!detailEntry) return
    setEditContent(detailEntry.content ?? '')
    setEditTags([...detailEntry.mood_tags])
    setEditError(null)
    setEditMode(true)
  }

  const toggleEditTag = (tag: string) => {
    setEditTags(prev =>
      prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    )
  }

  const handleSaveEdit = async (event: FormEvent) => {
    event.preventDefault()
    if (!detailEntry || editing) return
    if (!editContent.trim()) {
      setEditError('Please write something before saving.')
      return
    }
    setEditing(true)
    setEditError(null)
    try {
      const res = await api.journal.update(detailEntry.id, {
        content: editContent.trim(),
        mood_tags: editTags,
      })
      setDetailEntry(res.entry)
      setEntries(prev => prev.map(en => (en.id === res.entry.id ? res.entry : en)))
      setEditMode(false)
      setDetailNotice('Entry updated.')
    } catch (err) {
      setEditError(
        isApiError(err)
          ? err.message
          : 'We could not save your changes. Please try again.',
      )
    } finally {
      setEditing(false)
    }
  }

  const handleReflect = async (id: string) => {
    if (reflecting) return
    setReflecting(true)
    setReflectError(null)
    try {
      const res = await api.journal.reflect(id)
      setDetailEntry(res.entry)
      setEntries(prev => prev.map(en => (en.id === res.entry.id ? res.entry : en)))
      setDetailNotice('New reflection ready.')
    } catch (err) {
      setReflectError(
        isApiError(err) && err.code === 'AI_REFLECTION_UNAVAILABLE'
          ? 'The AI companion is temporarily unavailable. Your journal entry is safe.'
          : isApiError(err)
            ? err.message
            : 'We could not get a reflection right now. Please try again.',
      )
    } finally {
      setReflecting(false)
    }
  }

  const handleDelete = async () => {
    if (!detailEntry || deleting) return
    if (!deleteConfirm) {
      setDeleteConfirm(true)
      return
    }
    setDeleting(true)
    setDeleteError(null)
    try {
      await api.journal.delete(detailEntry.id)
      setEntries(prev => prev.filter(en => en.id !== detailEntry.id))
      setPageNotice('Entry deleted.')
      setDetailId(null)
      setDetailEntry(null)
      setEditMode(false)
      setDeleteConfirm(false)
      setDeleteError(null)
    } catch (err) {
      setDeleteError(
        isApiError(err) && err.status === 404
          ? 'This entry no longer exists. It may have been deleted already.'
          : isApiError(err)
            ? err.message
            : 'We could not delete this entry. Please try again.',
      )
    } finally {
      setDeleting(false)
    }
  }

  const handleLoadMore = async () => {
    if (!nextCursor || loadingMore) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const res = await api.journal.list({ limit: 20, before: nextCursor })
      setEntries(prev => [...prev, ...res.entries])
      setNextCursor(res.next_cursor)
    } catch (err) {
      setLoadMoreError(
        isApiError(err)
          ? err.message
          : 'We could not load more entries right now. Please try again.',
      )
    } finally {
      setLoadingMore(false)
    }
  }

  const editTagOptions = Array.from(new Set([...moodTags, ...editTags]))

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Your journal</h1>
        <p className={styles.sub}>Private only yours — <em>ya siri</em></p>
      </div>

      {/* Mode toggle */}
      <div className={styles.modeToggle} role="group" aria-label="Journal mode">
        <button
          className={[styles.modeBtn, mode === 'journal' ? styles.modeBtnActive : ''].join(' ')}
          onClick={() => switchMode('journal')}
        >
          ✍️ Journal
        </button>
        <button
          className={[styles.modeBtn, mode === 'prayer' ? styles.modeBtnActive : ''].join(' ')}
          onClick={() => switchMode('prayer')}
        >
          🙏 Prayer
        </button>
      </div>

      {/* ── JOURNAL MODE ── */}
      {mode === 'journal' && (
        <>
          <div className={styles.promptStrip} role="group" aria-label="Journal prompts">
            {prompts.map((p, i) => (
              <button
                key={p.label}
                className={[styles.promptChip, activePrompt === i ? styles.promptChipActive : ''].join(' ')}
                onClick={() => { setActivePrompt(i); textareaRef.current?.focus() }}
              >
                {p.label}
              </button>
            ))}
          </div>

          <div className={styles.writeBox}>
            <p className={styles.writeDate}>{today}</p>
            <textarea
              ref={textareaRef}
              className={styles.textarea}
              value={content}
              onChange={e => setContent(e.target.value)}
              placeholder={prompts[activePrompt].placeholder}
              aria-label="Journal entry"
              maxLength={10000}
              rows={6}
            />
            <div className={styles.writeFooter}>
              <span className={styles.wordCount}>
                {wordCount} {wordCount === 1 ? 'word' : 'words'}
              </span>
            </div>
          </div>

          <div>
            <p className={styles.sectionLabel}>How are you feeling?</p>
            <div className={styles.tagRow} role="group" aria-label="Mood tags">
              {moodTags.map(tag => (
                <button
                  key={tag}
                  className={[styles.tag, selectedTags.includes(tag) ? styles.tagActive : ''].join(' ')}
                  onClick={() => toggleTag(tag)}
                  aria-pressed={selectedTags.includes(tag)}
                >
                  {tag}
                </button>
              ))}
            </div>
          </div>
        </>
      )}

      {/* ── PRAYER MODE ── */}
      {mode === 'prayer' && (
        <div className={styles.prayerMode}>
          <p className={styles.prayerIntro}>
            "Do not be anxious about anything, but in every situation, by prayer and petition,
            with thanksgiving, present your requests to God." — Philippians 4:6
          </p>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="gratitude">
              🌿 Gratitude
              <span className={styles.prayerPrompt}>What am I thankful for today?</span>
            </label>
            <textarea
              id="gratitude"
              className={styles.prayerTextarea}
              value={gratitude}
              onChange={e => setGratitude(e.target.value)}
              placeholder="Lord, I am grateful for..."
              maxLength={10000}
              rows={3}
            />
          </div>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="petition">
              🕊️ Petition
              <span className={styles.prayerPrompt}>What am I bringing before God?</span>
            </label>
            <textarea
              id="petition"
              className={styles.prayerTextarea}
              value={petition}
              onChange={e => setPetition(e.target.value)}
              placeholder="Lord, I ask you for..."
              maxLength={10000}
              rows={3}
            />
          </div>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="listening">
              👂 Listening
              <span className={styles.prayerPrompt}>What do I feel He is saying to me?</span>
            </label>
            <textarea
              id="listening"
              className={styles.prayerTextarea}
              value={listening}
              onChange={e => setListening(e.target.value)}
              placeholder="I feel God is saying..."
              maxLength={10000}
              rows={3}
            />
          </div>

          <div className={styles.blessingCard}>
            <p className={styles.blessingEyebrow}>A word for you today</p>
            <blockquote className={styles.blessingText}>{blessing.text}</blockquote>
            <p className={styles.blessingRef}>{blessing.ref}</p>
          </div>
        </div>
      )}

      {/* Save button — both modes */}
      <button
        className={[styles.saveBtn, saving ? styles.saveBtnLoading : ''].join(' ')}
        onClick={handleSave}
        disabled={saving}
        aria-label="Save and get AI reflection"
      >
        {saving
          ? <><span className={styles.spinner} aria-hidden="true" /> Reflecting...</>
          : mode === 'prayer' ? '🙏 Save prayer' : 'Save & reflect with AI'
        }
      </button>

      {/* Create feedback + reflection */}
      {createError && <p className={styles.noticeError} role="alert">{createError}</p>}
      {createStatus && <p className={styles.noticeSuccess} role="status">{createStatus}</p>}
      {reflection && (
        <div className={styles.reflection} role="region" aria-live="polite">
          <p className={styles.reflectionEyebrow}>
            {mode === 'prayer' ? 'A word from your companion' : 'Your Soulwe companion'}
          </p>
          <p className={styles.reflectionText}>{reflection}</p>
        </div>
      )}
      {reflectionUnavailable && (
        <p className={styles.noticeInfo} role="status">
          Your entry is saved. The AI companion is temporarily unavailable right now.
        </p>
      )}

      {/* Past entries */}
      <div>
        <p className={styles.sectionLabel}>Past entries</p>

        {pageNotice && <p className={styles.noticeSuccess} role="status">{pageNotice}</p>}

        {/* Detail panel */}
        {detailId && (
          <section className={styles.detailCard} aria-label="Journal entry details">
            <div className={styles.detailTop}>
              <div className={styles.detailMeta}>
                <p className={styles.detailDate}>
                  {detailEntry ? formatFullDate(detailEntry.created_at) : ''}
                </p>
                {detailEntry && (
                  <p className={styles.detailWords}>
                    {wordsLabel(detailEntry.word_count)}
                    {detailEntry.prompt_used
                      ? ` · ${detailEntry.prompt_used === PRAYER_PROMPT ? 'Prayer' : detailEntry.prompt_used}`
                      : ''}
                  </p>
                )}
              </div>
              <button className={styles.secondaryBtn} onClick={closeDetail}>Close</button>
            </div>

            {detailLoading && <p className={styles.detailStatus}>Loading entry…</p>}

            {!detailLoading && detailError && (
              <div className={styles.noticeError} role="alert">
                {detailError}
                <button
                  className={styles.smallBtn}
                  onClick={() => detailId && openEntry(detailId)}
                >
                  Try again
                </button>
              </div>
            )}

            {!detailLoading && !detailError && detailEntry && (
              editMode ? (
                <form className={styles.editForm} onSubmit={handleSaveEdit}>
                  <textarea
                    className={styles.editTextarea}
                    value={editContent}
                    onChange={e => setEditContent(e.target.value)}
                    aria-label="Edit journal entry"
                    maxLength={10000}
                    rows={7}
                  />
                  <p className={styles.sectionLabel}>How are you feeling?</p>
                  <div className={styles.tagRow} role="group" aria-label="Mood tags">
                    {editTagOptions.map(tag => (
                      <button
                        key={tag}
                        type="button"
                        className={[styles.tag, editTags.includes(tag) ? styles.tagActive : ''].join(' ')}
                        onClick={() => toggleEditTag(tag)}
                        aria-pressed={editTags.includes(tag)}
                      >
                        {tag}
                      </button>
                    ))}
                  </div>
                  {editError && <p className={styles.noticeError} role="alert">{editError}</p>}
                  <div className={styles.editActions}>
                    <button className={styles.primaryBtn} type="submit" disabled={editing}>
                      {editing ? 'Saving…' : 'Save changes'}
                    </button>
                    <button
                      className={styles.secondaryBtn}
                      type="button"
                      onClick={() => setEditMode(false)}
                      disabled={editing}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <>
                  <div className={styles.entryContent}>{detailEntry.content}</div>
                  {detailEntry.mood_tags.length > 0 && (
                    <div className={styles.entryTags}>
                      {detailEntry.mood_tags.map(t => <span key={t} className={styles.entryTag}>{t}</span>)}
                    </div>
                  )}

                  <div className={styles.reflectionSection}>
                    {reflectError && <p className={styles.noticeError} role="alert">{reflectError}</p>}
                    {reflecting ? (
                      <p className={styles.reflectStatus}>
                        <span className={styles.spinner} aria-hidden="true" /> Asking your companion…
                      </p>
                    ) : detailEntry.ai_reflection ? (
                      <>
                        <p className={styles.reflectionEyebrow}>Your Soulwe companion</p>
                        <p className={styles.reflectionText}>{detailEntry.ai_reflection}</p>
                        <button
                          className={styles.reflectionBtn}
                          onClick={() => handleReflect(detailEntry.id)}
                        >
                          Fresh reflection
                        </button>
                      </>
                    ) : (
                      <div className={styles.noReflection}>
                        <p className={styles.noReflectionText}>No AI reflection yet.</p>
                        <button
                          className={styles.reflectionBtn}
                          onClick={() => handleReflect(detailEntry.id)}
                        >
                          Ask your companion
                        </button>
                      </div>
                    )}
                  </div>

                  {detailNotice && <p className={styles.noticeSuccess} role="status">{detailNotice}</p>}

                  <div className={styles.entryActions}>
                    <button className={styles.secondaryBtn} onClick={startEdit}>Edit</button>
                    {deleteConfirm ? (
                      <>
                        <button className={styles.dangerBtn} onClick={handleDelete} disabled={deleting}>
                          {deleting ? 'Deleting…' : 'Confirm delete'}
                        </button>
                        <button className={styles.secondaryBtn} onClick={() => setDeleteConfirm(false)}>
                          Keep entry
                        </button>
                      </>
                    ) : (
                      <button className={styles.dangerBtn} onClick={handleDelete}>Delete</button>
                    )}
                  </div>
                  {deleteError && <p className={styles.noticeError} role="alert">{deleteError}</p>}
                </>
              )
            )}
          </section>
        )}

        {listLoading ? (
          <p className={styles.listStatus}>Loading your journal…</p>
        ) : listError ? (
          <div className={styles.noticeError} role="alert">
            {listError}
            <button className={styles.smallBtn} onClick={() => setTick(t => t + 1)}>
              Try again
            </button>
          </div>
        ) : entries.length === 0 ? (
          <p className={styles.emptyNote}>Your journal is empty. Write your first entry above.</p>
        ) : (
          <>
            <div className={styles.entryList}>
              {entries.map(entry => {
                const d = formatListDate(entry.created_at)
                const isPrayer = entry.prompt_used === PRAYER_PROMPT
                return (
                  <button
                    key={entry.id}
                    className={[
                      styles.entryItem,
                      detailId === entry.id ? styles.entryItemActive : '',
                    ].join(' ')}
                    onClick={() => openEntry(entry.id)}
                    aria-label={`Open ${isPrayer ? 'prayer' : 'journal'} entry from ${formatFullDate(entry.created_at)}`}
                  >
                    <div className={styles.entryDate} aria-hidden="true">
                      <span className={styles.entryDay}>{d.day}</span>
                      <span className={styles.entryMonth}>{d.month}</span>
                    </div>
                    <div className={styles.entryBody}>
                      <div className={styles.entryMeta}>
                        <span className={styles.entryModeTag}>
                          {isPrayer ? '🙏 Prayer' : '✍️ Journal'}
                        </span>
                        <span className={styles.entryWords}>{wordsLabel(entry.word_count)}</span>
                        {entry.ai_reflection && (
                          <span className={styles.reflectionBadge}>Reflection</span>
                        )}
                      </div>
                      <p className={styles.entryPreview}>
                        {entry.prompt_used && entry.prompt_used !== PRAYER_PROMPT
                          ? `“${entry.prompt_used}”`
                          : 'Your private words — tap to read'}
                      </p>
                      {entry.mood_tags.length > 0 && (
                        <div className={styles.entryTags}>
                          {entry.mood_tags.map(t => <span key={t} className={styles.entryTag}>{t}</span>)}
                        </div>
                      )}
                    </div>
                  </button>
                )
              })}
            </div>
            {loadMoreError && <p className={styles.noticeError} role="alert">{loadMoreError}</p>}
            {nextCursor && (
              <button className={styles.loadMoreBtn} onClick={handleLoadMore} disabled={loadingMore}>
                {loadingMore ? 'Loading…' : 'Load older entries'}
              </button>
            )}
          </>
        )}
      </div>
    </div>
  )
}