import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { BookOpen, Users, Wind, UserCheck, CloudRain, Minus, TrendingUp, Leaf, Heart } from 'lucide-react'
import styles from './HomePage.module.css'

type Mood = 'Heavy' | 'Okay' | 'Better' | 'At peace' | 'Grateful'

const moods: { Icon: React.ElementType; label: Mood }[] = [
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
  { to: '/journal',   Icon: BookOpen,  label: 'Write in journal', sub: '3-day streak — keep going', color: 'clay'  },
  { to: '/circle',    Icon: Users,     label: 'Join a circle',    sub: '12 people online now',       color: 'sage'  },
  { to: '/breathe',   Icon: Wind,      label: 'Breathe',          sub: '2-minute calm reset',        color: 'earth' },
  { to: '/therapist', Icon: UserCheck, label: 'Find a therapist', sub: 'Starts from KES 500',        color: 'clay'  },
]

export default function HomePage() {
  const [mood, setMood] = useState<Mood | null>(null)
  const navigate = useNavigate()

  const affirmation = mood ? affirmations[mood] : affirmations['Grateful']

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