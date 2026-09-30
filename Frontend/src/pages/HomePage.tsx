import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { BookOpen, Users, Wind, UserCheck, CloudRain, Minus, TrendingUp, Leaf, Heart, type LucideIcon } from 'lucide-react'
import { api } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { isApiError, type DashboardResponse, type Mood, type MoodLog } from '@/types'
import styles from './HomePage.module.css'

const moods: { Icon: LucideIcon; label: Mood }[] = [
  { Icon: CloudRain, label: 'Heavy'    },
  { Icon: Minus,     label: 'Okay'     },
  { Icon: TrendingUp,label: 'Better'   },
  { Icon: Leaf,      label: 'At peace' },
  { Icon: Heart,     label: 'Grateful' },
]

const affirmations: Record<Mood, { text: string; attribution: string }> = {
  'Heavy': {
    text: '"Come to me, all you who are weary and burdened, and I will give you rest."',
    attribution: 'Matthew 11:28',
  },
  'Okay': {
    text: '"The Lord is my shepherd, I lack nothing. He makes me lie down in green pastures, he leads me beside quiet waters."',
    attribution: 'Psalm 23:1-2',
  },
  'Better': {
    text: '"I can do all this through him who gives me strength."',
    attribution: 'Philippians 4:13',
  },
  'At peace': {
    text: '"And the peace of God, which transcends all understanding, will guard your hearts and your minds in Christ Jesus."',
    attribution: 'Philippians 4:7',
  },
  'Grateful': {
    text: '"Give thanks to the Lord, for he is good; his love endures forever."',
    attribution: 'Psalm 107:1',
  },
}

const quickCards = [
  { to: '/journal',   Icon: BookOpen,  label: 'Write in journal', sub: 'Your private space', color: 'clay'  },
  { to: '/circle',    Icon: Users,     label: 'Join a circle',    sub: 'Talk with others',    color: 'sage'  },
  { to: '/breathe',   Icon: Wind,      label: 'Breathe',          sub: 'Calm reset',          color: 'earth' },
  { to: '/therapist', Icon: UserCheck, label: 'Find a therapist', sub: 'Book a session',      color: 'clay'  },
]

function formatLoggedAt(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

export default function HomePage() {
  const navigate = useNavigate()
  const { status } = useAuth()
  // Browsing is always allowed. This page is complete for a guest: the
  // greeting, affirmation and quick links are all public, and so is checking in
  // — a guest's check-ins belong to their anonymous session. Only the account
  // dashboard, which is built around a profile, needs an account.
  const hasAccount = status === 'authenticated'

  const [dashboard, setDashboard] = useState<DashboardResponse | null>(null)
  const [checkins, setCheckins] = useState<MoodLog[]>([])
  const [activeMood, setActiveMood] = useState<Mood | null>(null)
  const [saving, setSaving] = useState(false)
  const [savedMood, setSavedMood] = useState<Mood | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [feedError, setFeedError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    setFeedError(null)
    // The dashboard carries the user's profile, so it is registered-only. The
    // check-in history is not: a guest gets their own, anonymous one.
    const feed = hasAccount
      ? Promise.all([api.dashboard.get(), api.moods.list({ limit: 12 })]).then(([dash, moodsRes]) => ({
          dash,
          checkins: moodsRes.moods,
        }))
      : api.moods
          .list({ limit: 12 })
          .then((moodsRes) => ({ dash: null, checkins: moodsRes.moods }))

    feed
      .then(({ dash, checkins: loaded }) => {
        if (cancelled) return
        setDashboard(dash)
        setCheckins(loaded)
        const latest = dash?.latest_mood ?? loaded[0]
        if (latest) setActiveMood(prev => prev ?? (latest.mood as Mood))
      })
      .catch(err => {
        if (cancelled) return
        setFeedError(
          isApiError(err)
            ? err.message
            : 'There was a problem loading your space. Please try again.',
        )
      })
    return () => {
      cancelled = true
    }
  }, [tick, hasAccount])

  const handleMoodSelect = async (label: Mood) => {
    if (saving) return
    setActiveMood(label)
    setSaveError(null)
    setSavedMood(null)
    setSaving(true)
    try {
      const log = await api.moods.create({ mood: label })
      setCheckins(prev => [log, ...prev])
      setSavedMood(label)
      setDashboard(prev =>
        prev
          ? {
              ...prev,
              latest_mood: log,
              recent_moods: [log, ...prev.recent_moods].slice(0, 5),
              mood_checkins_count: prev.mood_checkins_count + 1,
            }
          : prev,
      )
    } catch (err) {
      setSaveError(
        isApiError(err)
          ? err.message
          : 'We could not save your mood right now. Please try again.',
      )
    } finally {
      setSaving(false)
    }
  }

  const affirmation = activeMood ? affirmations[activeMood] : affirmations['Grateful']
  const member = dashboard?.user ?? null
  const checkinCount = dashboard?.mood_checkins_count ?? checkins.length
  const latestMood = checkins[0] ?? null
  const lastCheckin = latestMood ? formatLoggedAt(latestMood.logged_at) : '—'
  const recent = checkins.slice(0, 5)

  // A failed dashboard load replaces the page, because the page's own subject
  // is the account. A guest has no dashboard: their check-in history is a
  // secondary detail, so a failure there stays inline and the affirmation and
  // quick links — which need no credential at all — keep working.
  if (feedError && hasAccount && !dashboard) {
    return (
      <div className={styles.page}>
        <section className={styles.errorPanel} role="alert">
          <p className={styles.errorEyebrow}>Something went wrong</p>
          <p className={styles.errorText}>{feedError}</p>
          <button className={styles.retryBtn} onClick={() => setTick(t => t + 1)}>
            Try again
          </button>
        </section>
      </div>
    )
  }

  return (
    <div className={styles.page}>

      {/* Greeting band */}
      <section className={styles.greetBand} aria-label="Welcome">
        <p className={styles.greetEyebrow}>
          {member && member.display_name
            ? `Karibu, ${member.display_name}`
            : 'Karibu — Welcome'}
        </p>
        <h1 className={styles.greetHeading}>
          Nafsi yangu,<br />
          <em>how are you today?</em>
        </h1>
        <p className={styles.greetSub}>
          {hasAccount
            ? 'This is your private space. No judgment, no stigma — just you.'
            : 'You are welcome here, with or without an account. Take your time — everything on this page is yours to explore.'}
        </p>

        {/* Mood picker — always browsable; only saving a check-in needs an account */}
        <div className={styles.moodRow} role="group" aria-label="How are you feeling?">
          {moods.map(({ Icon, label }) => (
            <button
              key={label}
              className={[styles.moodBtn, activeMood === label ? styles.moodBtnActive : ''].join(' ')}
              onClick={() => handleMoodSelect(label)}
              disabled={saving}
              aria-pressed={activeMood === label}
              aria-label={`Feeling ${label}`}
            >
              <span className={styles.moodEmoji} aria-hidden="true">
                <Icon size={20} strokeWidth={1.8} />
              </span>
              <span className={styles.moodLabel}>{label}</span>
            </button>
          ))}
        </div>

        {/* Mood save status */}
        {(saving || savedMood !== null || saveError) && (
          <p
            className={saveError ? styles.moodError : styles.moodSaved}
            role={saveError ? 'alert' : 'status'}
          >
            {saving ? 'Saving your mood…' : saveError ?? 'Saved — how you are feeling today.'}
          </p>
        )}
      </section>

      {/* Affirmation */}
      <section className={styles.affirmation} aria-label="Today's affirmation">
        <p className={styles.affirmationEyebrow}>Today's affirmation</p>
        <blockquote className={styles.affirmationText}>
          {affirmation.text}
        </blockquote>
        <p className={styles.affirmationSource}>{affirmation.attribution}</p>
      </section>

      {/* Quick access */}
      <section aria-label="Quick access">
        <p className={styles.sectionLabel}>Your space</p>
        <div className={styles.quickGrid}>
          {quickCards.map(({ to, Icon, label, sub, color }) => (
            <button
              key={to}
              className={[styles.quickCard, styles[`quickCard_${color}`]].join(' ')}
              onClick={() => navigate(to)}
              aria-label={label}
            >
              <span className={styles.quickIcon} aria-hidden="true">
                <Icon size={22} strokeWidth={1.8} />
              </span>
              <span className={styles.quickLabel}>{label}</span>
              <span className={styles.quickSub}>{sub}</span>
            </button>
          ))}
        </div>
      </section>

      {/* A quiet, voluntary way in — not a gate. Browsing and checking in both
          work for a guest; an account only adds a dashboard and cross-device
          history. */}
      {!hasAccount && (
        <p className={styles.guestNote}>
          You are using Soulwe as a guest — your journal, check-ins and breathing history
          are saved to this browser. Circles, breathing and therapist discovery are all
          open to you.{' '}
          <Link className={styles.greetLink} to="/register" state={{ from: '/home' }}>
            Create a free account
          </Link>{' '}
          whenever you want your history to follow you, or{' '}
          <Link className={styles.greetLink} to="/login" state={{ from: '/home' }}>
            log in
          </Link>
          {' '}if you already have one.
        </p>
      )}

      {/* Activity summary — the caller's own history. A guest has one too, under
          their anonymous session, so this is not account-gated. */}
      {feedError && !hasAccount && (
        <section className={styles.errorPanel} role="alert">
          <p className={styles.errorText}>
            We couldn’t load your check-in history — {feedError}
          </p>
          <button className={styles.retryBtn} onClick={() => setTick(t => t + 1)}>
            Try again
          </button>
        </section>
      )}
      {!feedError && (
        <>
          <section aria-label="Your activity">
            <div className={styles.statsRow}>
              <div className={styles.statItem}>
                <span className={styles.statNum}>{checkinCount}</span>
                <span className={styles.statLabel}>Mood check-ins</span>
              </div>
              <div className={styles.statDivider} aria-hidden="true" />
              <div className={styles.statItem}>
                <span className={styles.statNum}>{latestMood ? latestMood.mood : '—'}</span>
                <span className={styles.statLabel}>Latest mood</span>
              </div>
              <div className={styles.statDivider} aria-hidden="true" />
              <div className={styles.statItem}>
                <span className={styles.statNum}>{lastCheckin}</span>
                <span className={styles.statLabel}>Last check-in</span>
              </div>
            </div>
          </section>

          {/* Recent check-ins */}
          <section aria-label="Recent check-ins">
            <p className={styles.sectionLabel}>Recent check-ins</p>
            {recent.length === 0 ? (
              <p className={styles.emptyNote}>
                No check-ins yet — tap a mood above to start.
              </p>
            ) : (
              <ul className={styles.checkinList}>
                {recent.map(log => (
                  <li key={log.id} className={styles.checkinItem}>
                    <span className={styles.checkinMood}>{log.mood}</span>
                    <span className={styles.checkinDate}>{formatLoggedAt(log.logged_at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      )}

    </div>
  )
}