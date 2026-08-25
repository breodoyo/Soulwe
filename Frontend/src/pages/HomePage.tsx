import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import styles from './HomePage.module.css'

type Mood = 'Heavy' | 'Okay' | 'Better' | 'At peace' | 'Grateful'

const moods: { emoji: string; label: Mood }[] = [
  { emoji: '😔', label: 'Heavy'    },
  { emoji: '😐', label: 'Okay'     },
  { emoji: '🙂', label: 'Better'   },
  { emoji: '😌', label: 'At peace' },
  { emoji: '✨', label: 'Grateful' },
]

const affirmations: Record<Mood, { text: string; attribution: string }> = {
  'Heavy': {
    text: '"Haba na haba hujaza kibaba" — Little by little fills the measure. You are still moving forward, even when it does not feel like it.',
    attribution: 'Swahili proverb',
  },
  'Okay': {
    text: '"Pole pole ndio mwendo" — Slowly slowly is the way. Steady and present is more than enough for today.',
    attribution: 'Swahili proverb',
  },
  'Better': {
    text: '"Baada ya dhiki faraja" — After hardship comes relief. You are already on the way up.',
    attribution: 'Swahili proverb',
  },
  'At peace': {
    text: '"Amani ya moyo ni utajiri" — Peace of the heart is true wealth. Rest in this moment. You have earned it.',
    attribution: 'Swahili proverb',
  },
  'Grateful': {
    text: '"Shukrani ni daraja" — Gratitude is a bridge. You are connected to something larger than your worries.',
    attribution: 'Swahili proverb',
  },
}

const quickCards = [
  { to: '/journal',   icon: '✍️', label: 'Write in journal', sub: '3-day streak — keep going',   color: 'clay'  },
  { to: '/circle',    icon: '🌿', label: 'Join a circle',     sub: '12 people online now',         color: 'sage'  },
  { to: '/breathe',   icon: '🫁', label: 'Breathe',           sub: '2-minute calm reset',          color: 'earth' },
  { to: '/therapist', icon: '🤝', label: 'Find a therapist',  sub: 'Starts from KES 500',          color: 'clay'  },
]

export default function HomePage() {
  const [mood, setMood] = useState<Mood | null>(null)
  const navigate = useNavigate()

  const affirmation = mood ? affirmations[mood] : affirmations['Okay']

  return (
    <div className={styles.page}>

      {/* Greeting band */}
      <section className={styles.greetBand} aria-label="Welcome">
        <p className={styles.greetEyebrow}>Karibu — Welcome back</p>
        <h1 className={styles.greetHeading}>
          Nafsi yangu,<br />
          <em>how are you today?</em>
        </h1>
        <p className={styles.greetSub}>
          This is your private space. No judgment, no stigma — just you.
        </p>

        {/* Mood picker */}
        <div className={styles.moodRow} role="group" aria-label="How are you feeling?">
          {moods.map(({ emoji, label }) => (
            <button
              key={label}
              className={[styles.moodBtn, mood === label ? styles.moodBtnActive : ''].join(' ')}
              onClick={() => setMood(label)}
              aria-pressed={mood === label}
              aria-label={`Feeling ${label}`}
            >
              <span className={styles.moodEmoji} aria-hidden="true">{emoji}</span>
              <span className={styles.moodLabel}>{label}</span>
            </button>
          ))}
        </div>
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
          {quickCards.map(({ to, icon, label, sub, color }) => (
            <button
              key={to}
              className={[styles.quickCard, styles[`quickCard_${color}`]].join(' ')}
              onClick={() => navigate(to)}
              aria-label={label}
            >
              <span className={styles.quickIcon} aria-hidden="true">{icon}</span>
              <span className={styles.quickLabel}>{label}</span>
              <span className={styles.quickSub}>{sub}</span>
            </button>
          ))}
        </div>
      </section>

      {/* Streak / stats */}
      <section className={styles.statsRow} aria-label="Your activity">
        <div className={styles.statItem}>
          <span className={styles.statNum}>3</span>
          <span className={styles.statLabel}>Day streak</span>
        </div>
        <div className={styles.statDivider} aria-hidden="true" />
        <div className={styles.statItem}>
          <span className={styles.statNum}>7</span>
          <span className={styles.statLabel}>Journal entries</span>
        </div>
        <div className={styles.statDivider} aria-hidden="true" />
        <div className={styles.statItem}>
          <span className={styles.statNum}>4</span>
          <span className={styles.statLabel}>Breathe sessions</span>
        </div>
      </section>

    </div>
  )
}
