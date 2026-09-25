import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { isApiError, type Booking, type Therapist } from '@/types'
import styles from './TherapistPage.module.css'

// Filters available in the UI. Language pills map to the backend ?language
// query param (server-side substring match); "Online only" has no backend
// equivalent, so it filters the already-fetched list client-side. There is no
// "free sessions" filter — the backend deliberately never exposes that field.
type Filter = 'All' | 'Swahili' | 'Dholuo' | 'Kikuyu' | 'Online only'
const FILTERS: Filter[] = ['All', 'Swahili', 'Dholuo', 'Kikuyu', 'Online only']

const AVATAR_COLORS = ['#C4714A', '#4A6741', '#5C3D2E', '#B45309', '#7C6A4D', '#A16207']

function initialsOf(name: string): string {
  return name
    .split(/\s+/)
    .map(w => w[0])
    .filter(Boolean)
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

// Deterministic warm colour per therapist, so a card always looks the same.
function avatarColor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  }
  return AVATAR_COLORS[hash % AVATAR_COLORS.length]
}

function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('en-KE', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

// The <input type="datetime-local"> minimum: the current local time, truncated
// to the minute.
function nowLocalInput(): string {
  const d = new Date()
  d.setSeconds(0, 0)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function bookingErrorMessage(err: unknown): string {
  if (!isApiError(err)) return 'We could not book this session. Please try again.'
  switch (err.code) {
    case 'BOOKING_CONFLICT':
      return 'That time is no longer available. Please choose another time.'
    case 'THERAPIST_UNAVAILABLE':
      return 'This therapist is not accepting bookings right now.'
    case 'NOT_FOUND':
      return 'This therapist is no longer available.'
    default:
      if (err.status === 400) {
        return err.field === 'scheduled_at'
          ? 'Please choose a date and time in the future.'
          : err.message
      }
      return err.message
  }
}

export default function TherapistPage() {
  const [filter, setFilter] = useState<Filter>('All')
  const [therapists, setTherapists] = useState<Therapist[] | null>(null)
  const [therapistsError, setTherapistsError] = useState<string | null>(null)
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadTick, setLoadTick] = useState(0)

  const [bookings, setBookings] = useState<Booking[] | null>(null)
  const [bookingsError, setBookingsError] = useState<string | null>(null)
  const [bookingsTick, setBookingsTick] = useState(0)

  const [openBookingId, setOpenBookingId] = useState<string | null>(null)
  const [slot, setSlot] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [bookingError, setBookingError] = useState<string | null>(null)
  const [successById, setSuccessById] = useState<Record<string, string>>({})

  const [cancelConfirmId, setCancelConfirmId] = useState<string | null>(null)
  const [cancellingId, setCancellingId] = useState<string | null>(null)
  const [cancelError, setCancelError] = useState<string | null>(null)

  const languageParam = filter === 'All' || filter === 'Online only' ? undefined : filter

  useEffect(() => {
    let cancelled = false
    setTherapists(null)
    setTherapistsError(null)
    setNextCursor(null)
    api.therapists
      .list({ language: languageParam })
      .then(res => {
        if (cancelled) return
        setTherapists(res.therapists)
        setNextCursor(res.next_cursor)
      })
      .catch(err => {
        if (cancelled) return
        setTherapistsError(
          isApiError(err) ? err.message : 'We could not load therapists right now. Please try again.',
        )
      })
    return () => {
      cancelled = true
    }
  }, [languageParam, loadTick])

  useEffect(() => {
    let cancelled = false
    setBookings(null)
    setBookingsError(null)
    api.bookings
      .list()
      .then(res => {
        if (cancelled) return
        setBookings(res.bookings)
      })
      .catch(err => {
        if (cancelled) return
        setBookingsError(
          isApiError(err) ? err.message : 'We could not load your bookings right now. Please try again.',
        )
      })
    return () => {
      cancelled = true
    }
  }, [bookingsTick])

  // "Online only" is applied client-side: the backend returns is_online_only
  // but provides no filter for it.
  const visible =
    therapists === null
      ? null
      : filter === 'Online only'
        ? therapists.filter(t => t.is_online_only)
        : therapists

  const handleLoadMore = async () => {
    if (!nextCursor || loadingMore) return
    setLoadingMore(true)
    try {
      const res = await api.therapists.list({ language: languageParam, before: nextCursor })
      setTherapists(prev => [...(prev ?? []), ...res.therapists])
      setNextCursor(res.next_cursor)
    } catch (err) {
      setTherapistsError(
        isApiError(err) ? err.message : 'We could not load more therapists right now. Please try again.',
      )
    } finally {
      setLoadingMore(false)
    }
  }

  const handleBook = async (t: Therapist) => {
    if (!slot || submitting) return
    const sched = new Date(slot)
    if (Number.isNaN(sched.getTime()) || sched.getTime() <= Date.now()) {
      setBookingError('Please choose a date and time in the future.')
      return
    }
    setSubmitting(true)
    setBookingError(null)
    try {
      const res = await api.bookings.create(t.id, { scheduled_at: sched.toISOString() })
      setSuccessById(prev => ({ ...prev, [t.id]: formatDateTime(res.booking.scheduled_at) }))
      setOpenBookingId(null)
      setSlot('')
      setBookingsTick(n => n + 1)
    } catch (err) {
      setBookingError(bookingErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const handleCancel = async (b: Booking) => {
    if (cancellingId) return
    if (cancelConfirmId !== b.id) {
      setCancelConfirmId(b.id)
      setCancelError(null)
      return
    }
    setCancellingId(b.id)
    setCancelError(null)
    try {
      const res = await api.bookings.cancel(b.id)
      setBookings(prev => (prev ? prev.map(x => (x.id === b.id ? res.booking : x)) : prev))
    } catch (err) {
      if (isApiError(err) && err.code === 'BOOKING_STATUS_CONFLICT') {
        setCancelError('This booking can no longer be cancelled.')
        setBookingsTick(n => n + 1)
      } else {
        setCancelError(
          isApiError(err) ? err.message : 'We could not cancel this booking. Please try again.',
        )
      }
    } finally {
      setCancellingId(null)
      setCancelConfirmId(null)
    }
  }

  const statusLabel: Record<Booking['status'], string> = {
    pending: 'Pending',
    confirmed: 'Confirmed',
    cancelled: 'Cancelled',
    completed: 'Completed',
  }

  const statusChipClass: Record<Booking['status'], string> = {
    pending: styles.chipPending,
    confirmed: styles.chipConfirmed,
    cancelled: styles.chipCancelled,
    completed: styles.chipCompleted,
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Find a therapist</h1>
        <p className={styles.sub}>African therapists who understand your world — <em>washauri wetu</em></p>
      </div>

      {/* Filters */}
      <div className={styles.filterRow} role="group" aria-label="Filter therapists">
        {FILTERS.map(f => (
          <button
            key={f}
            className={[styles.filterPill, filter === f ? styles.filterPillActive : ''].join(' ')}
            onClick={() => setFilter(f)}
            aria-pressed={filter === f}
          >
            {f}
          </button>
        ))}
      </div>

      {/* Cards */}
      <div className={styles.cardList}>
        {visible === null && !therapistsError && (
          <p className={styles.status} role="status">Loading therapists…</p>
        )}

        {therapistsError && (
          <div className={styles.errorNote} role="alert">
            <p>{therapistsError}</p>
            <button className={styles.inlineBtn} onClick={() => setLoadTick(n => n + 1)}>
              Try again
            </button>
          </div>
        )}

        {visible !== null && !therapistsError && visible.length === 0 && (
          <div className={styles.empty}>
            <p>No therapists match this filter.</p>
            <button className={styles.clearFilter} onClick={() => setFilter('All')}>
              Show all therapists
            </button>
          </div>
        )}

        {visible !== null &&
          !therapistsError &&
          visible.map(t => (
            <div key={t.id} className={styles.card}>
              <div className={styles.cardTop}>
                <div className={styles.avatar} style={{ background: avatarColor(t.display_name) }}>
                  {initialsOf(t.display_name)}
                </div>
                <div className={styles.info}>
                  <h2 className={styles.name}>{t.display_name}</h2>
                  {t.bio && <p className={styles.bio}>{t.bio}</p>}
                  {t.is_online_only && (
                    <p className={styles.location}>
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>
                      Online only
                    </p>
                  )}
                </div>
              </div>

              {(t.specialties.length > 0 || t.languages.length > 0) && (
                <div className={styles.tags}>
                  {t.specialties.map(s => <span key={s} className={styles.tag}>{s}</span>)}
                  {t.languages.map(l => <span key={l} className={[styles.tag, styles.tagLang].join(' ')}>{l}</span>)}
                </div>
              )}

              {t.session_price != null && (
                <div className={styles.priceRow}>
                  <span className={styles.price}>
                    {t.currency} {t.session_price.toLocaleString()}/session
                  </span>
                </div>
              )}

              <div className={styles.cardBottom}>
                <button
                  className={[styles.bookBtn, successById[t.id] ? styles.bookBtnBooked : ''].join(' ')}
                  onClick={() => setOpenBookingId(openBookingId === t.id ? null : t.id)}
                  aria-label={`Book session with ${t.display_name}`}
                  aria-expanded={openBookingId === t.id}
                >
                  {successById[t.id] ? 'Book another time' : 'Book session'}
                </button>
              </div>

              {successById[t.id] && (
                <p className={styles.bookConfirm} role="status">
                  ✓ Booked for {successById[t.id]} — pending confirmation.
                </p>
              )}

              {openBookingId === t.id && (
                <div className={styles.bookPanel}>
                  <label className="sr-only" htmlFor={`slot-${t.id}`}>
                    Choose a time for your session with {t.display_name}
                  </label>
                  <input
                    id={`slot-${t.id}`}
                    className={styles.slotInput}
                    type="datetime-local"
                    min={nowLocalInput()}
                    value={slot}
                    onChange={e => setSlot(e.target.value)}
                  />
                  {bookingError && <p className={styles.errorText} role="alert">{bookingError}</p>}
                  <div className={styles.bookActions}>
                    <button
                      className={styles.confirmBtn}
                      onClick={() => void handleBook(t)}
                      disabled={submitting || !slot}
                    >
                      {submitting
                        ? <span className={styles.bookSpinner} aria-hidden="true" />
                        : 'Confirm booking'}
                    </button>
                    <button
                      className={styles.inlineBtn}
                      onClick={() => {
                        setOpenBookingId(null)
                        setBookingError(null)
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}

        {visible !== null && !therapistsError && nextCursor && (
          <button
            className={styles.loadMoreBtn}
            onClick={() => void handleLoadMore()}
            disabled={loadingMore}
          >
            {loadingMore ? 'Loading…' : 'Load more therapists'}
          </button>
        )}
      </div>

      {/* My bookings */}
      <div className={styles.bookingsSection}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>My bookings</h2>
          {cancelError && <p className={styles.errorText} role="alert">{cancelError}</p>}
        </div>

        {bookings === null && !bookingsError && (
          <p className={styles.status} role="status">Loading your bookings…</p>
        )}

        {bookingsError && (
          <div className={styles.errorNote} role="alert">
            <p>{bookingsError}</p>
            <button className={styles.inlineBtn} onClick={() => setBookingsTick(n => n + 1)}>
              Try again
            </button>
          </div>
        )}

        {bookings !== null && !bookingsError && bookings.length === 0 && (
          <p className={styles.emptyNote}>You have no bookings yet. Book a session above.</p>
        )}

        {bookings !== null && !bookingsError && bookings.length > 0 && (
          <div className={styles.bookingList}>
            {bookings.map(b => (
              <div key={b.id} className={styles.bookingRow}>
                <div className={styles.bookingMain}>
                  <p className={styles.bookingTherapist}>{b.display_name}</p>
                  <p className={styles.bookingTime}>{formatDateTime(b.scheduled_at)}</p>
                </div>
                <span className={[styles.statusChip, statusChipClass[b.status]].join(' ')}>
                  {statusLabel[b.status]}
                </span>
                {b.status === 'pending' && (
                  <button
                    className={styles.cancelBtn}
                    onClick={() => void handleCancel(b)}
                    disabled={cancellingId === b.id}
                    aria-label={`Cancel booking with ${b.display_name}`}
                  >
                    {cancelConfirmId === b.id
                      ? (cancellingId === b.id ? 'Cancelling…' : 'Confirm cancel?')
                      : 'Cancel'}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}