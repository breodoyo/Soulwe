import { useEffect, useState, type KeyboardEvent } from 'react'
import { Link } from 'react-router-dom'
import { PenLine, Leaf, Handshake, Wind, ArrowRight } from 'lucide-react'
import { api } from '@/lib/api'
import { statsSourceUrl } from '@/landingContent'
import { moodVerses } from '@/moodVerses'
import { dailyVerses, dailyVerseIndex } from '@/dailyVerses'
import type { Therapist } from '@/types'
import styles from './LandingPage.module.css'
import '@/styles/sunrise.css'

// TODO(real data): confirm GET /api/v1/therapists returns rows in the deployed
// environment before relying on this. While it is on, the therapist section
// renders the real directory whenever the public API returns any, and a plain
// "Therapists are joining" state otherwise. Turn it off to force the placeholder.
const SHOW_THERAPIST_DIRECTORY = true

const features = [
  {
    icon: PenLine,
    accent: 'sun',
    title: 'A private journal',
    desc: 'Write freely, in your own words. Stored encrypted and never shown to other people.',
    link: { to: '/journal', label: 'Open your journal' },
    extraLink: { to: '/privacy', label: 'How your entry is handled' },
  },
  {
    icon: Leaf,
    accent: 'terracotta',
    title: 'Peer support circles',
    desc: 'Anonymous groups for grief, work, family and faith. A different anonymous name in every circle.',
    link: { to: '/circles', label: 'Browse circles' },
  },
  {
    icon: Handshake,
    accent: 'leaf',
    title: 'Talk to a therapist',
    desc: 'Kenyan therapists are joining soon, with sessions in several local languages.',
    link: { to: '/therapists', label: "See what's coming" },
  },
  {
    icon: Wind,
    accent: 'ink',
    title: 'Guided breathing',
    desc: 'Short, guided exercises for whenever you need a moment to settle.',
    link: { to: '/breathe', label: 'Try breathing' },
  },
]

// One accent colour per feature card; the classes live in the module. Icons are
// decorative (aria-hidden), so the pairings are stylistic, not text contrast.
const featureAccentClass: Record<string, string> = {
  sun: styles.featureIconSun,
  terracotta: styles.featureIconTerracotta,
  leaf: styles.featureIconLeaf,
  ink: styles.featureIconInk,
}

// The eight circles are the ones seeded by migrations 010 and 020, not made up
// for this page. TODO(real data): fetch this list from GET /circles once a
// public, token-less circles endpoint exists, so the page can never drift from
// the database.
const circleTopics = [
  'Grief & Loss',
  'Work & Pressure',
  'Family',
  'Relationships',
  'Young Adult Identity',
  'Trauma & Healing',
  'Faith & Doubt',
  'Anxiety & Depression',
]

function formatPrice(t: Therapist): string {
  if (t.session_price == null) return 'Price on request'
  return `${t.currency} ${t.session_price}`
}

// Verse layout is handled by the panel; no extra sizer needed.

// Preference for the hero check-in: faith is optional, so Scripture can be
// hidden. Verses are always ON by default. The key is versioned (now v3): a
// value stored by an earlier version (v1, v2) can never keep them hidden,
// because we only ever read the current key. Nothing is written to storage
// until the user actually clicks the toggle. Every access is wrapped for when
// storage is unavailable (private mode, blocked cookies).
const SCRIPTURE_KEY = 'soulwe.scripture.v3'

export default function LandingPage() {
  const [therapists, setTherapists] = useState<Therapist[]>([])
  const [heroMood, setHeroMood] = useState<string | null>(null)
  const [hideScripture, setHideScripture] = useState(() => {
    try {
      return window.localStorage.getItem(SCRIPTURE_KEY) === 'hidden'
    } catch {
      return false
    }
  })

  const activeVerse =
    heroMood !== null
      ? moodVerses.find(v => v.mood === heroMood)
      : undefined

  // Today's affirmation card: one verse for the whole day, picked from the
  // local date. If it happens to match the verse shown for the selected mood,
  // step to the next one so the same verse never appears twice in the hero.
  const dailyIndex = dailyVerseIndex()
  let cardVerse = dailyVerses[dailyIndex]
  if (activeVerse && activeVerse.reference === cardVerse.reference) {
    cardVerse = dailyVerses[(dailyIndex + 1) % dailyVerses.length]
  }

  const toggleScripture = () => {
    setHideScripture(prev => {
      const next = !prev
      try {
        window.localStorage.setItem(SCRIPTURE_KEY, next ? 'hidden' : 'visible')
      } catch {
        /* storage unavailable: the choice still applies for this visit */
      }
      return next
    })
  }

  // Radiogroup keyboard support: arrows move and select, roving tabindex.
  const onMoodKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const count = moodVerses.length
    let next: number | null = null
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown') next = (index + 1) % count
    else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') next = (index - 1 + count) % count
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = count - 1
    if (next === null) return
    event.preventDefault()
    setHeroMood(moodVerses[next].mood)
    const group = event.currentTarget.parentElement
    const radios = group?.querySelectorAll<HTMLButtonElement>('[role="radio"]')
    radios?.[next]?.focus()
  }

  useEffect(() => {
    if (!SHOW_THERAPIST_DIRECTORY) return
    let cancelled = false
    // Public endpoint, no token. Any failure leaves the placeholder state up,
    // so a down API never invents a therapist on the landing page.
    api.therapists
      .list()
      .then(res => {
        if (!cancelled) setTherapists(res.therapists)
      })
      .catch(() => {
        /* keep the placeholder */
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className={`${styles.page} sunrise`}>

      {/* ── Hero: full-bleed hero.jpg. ONE flat sun overlay at 75% covers the
             whole photo, edge to edge, at the same opacity everywhere (no
             gradient, scrim, blur or glow). Ink copy over the composite holds
             at least 4.8:1, even over the darkest foreground pixels. ── */}
      <section className={styles.hero}>
        <img
          className={styles.heroImg}
          src="/hero.jpg"
          alt="Two women sit close together in tall golden grass, eyes closed, resting their heads together"
          width={5760}
          height={3840}
          fetchPriority="high"
          decoding="async"
        />
        {/* Flat sun overlay edge to edge: decorative and behind the copy. */}
        <div className={styles.heroOverlay} aria-hidden="true" />
        <div className={styles.heroLayer}>
          <div className={styles.inner}>
            <div className={styles.heroText}>
              <div className={styles.heroPanel}>
                <h1 className={styles.heroHeading}>
                  Your soul deserves the{' '}
                  <span className={styles.heroAccent}>same care</span> as your
                  body.
                </h1>

                <p className={styles.heroSub}>
                  {therapists.length > 0 ? (
                    <>
                      Peer support circles, a private journal, and Kenyan
                      therapists who work in Swahili, Dholuo and Kikuyu.
                    </>
                  ) : (
                    <>
                      Peer support circles, a private journal, and Kenyan
                      therapists joining soon, in Swahili, Dholuo and Kikuyu.
                    </>
                  )}
                </p>

                <div className={styles.moodCheck}>
                  <div className={styles.moodHead}>
                    <p className={styles.moodLabel} id="mood-question">
                      How is your soul today?
                    </p>
                    <button
                      type="button"
                      className={styles.moodToggle}
                      aria-pressed={hideScripture}
                      onClick={toggleScripture}
                    >
                      {hideScripture ? 'Show Scripture' : 'Hide Scripture'}
                    </button>
                  </div>
                  <div
                    className={styles.moodRow}
                    role="radiogroup"
                    aria-labelledby="mood-question"
                  >
                    {moodVerses.map((verse, index) => (
                      <button
                        key={verse.mood}
                        type="button"
                        role="radio"
                        aria-checked={heroMood === verse.mood}
                        tabIndex={
                          heroMood === null
                            ? index === 0
                              ? 0
                              : -1
                            : heroMood === verse.mood
                            ? 0
                            : -1
                        }
                        className={[
                          styles.moodBtn,
                          heroMood === verse.mood ? styles.moodBtnOn : '',
                        ].join(' ')}
                        onClick={() => setHeroMood(verse.mood)}
                        onKeyDown={event => onMoodKeyDown(event, index)}
                      >
                        {verse.mood}
                      </button>
                    ))}
                  </div>
                  <div className={styles.verseBlock} aria-live="polite">
                    <div className={styles.verseContent}>
                      {hideScripture || activeVerse === undefined ? (
                        <p className={styles.versePlain}>
                          Rest is welcome here. Stay as long as you like.
                        </p>
                      ) : (
                        <>
                          <p className={styles.verseText}>
                            &ldquo;{activeVerse.text}&rdquo;
                          </p>
                          <p className={styles.verseRef}>
                            {activeVerse.reference} ({activeVerse.translation})
                          </p>
                        </>
                      )}
                    </div>
                  </div>
                </div>

                {heroMood === 'Heavy' && (
                  <p className={styles.verseHelp}>
                    If today feels like too much,{' '}
                    <a className={styles.verseHelpLink} href="#crisis">
                      help is here
                    </a>
                    .
                  </p>
                )}

                <div className={styles.heroActions}>
                  <Link className={styles.btnPrimary} to="/home">
                    Enter Soulwe
                  </Link>
                  <a className={styles.textLink} href="#features">
                    See how it works
                  </a>
                </div>
              </div>

              {heroMood === 'Heavy' && (
                <p className={styles.verseHelpBelow}>
                  If today feels like too much,{' '}
                  <a className={styles.verseHelpLink} href="#crisis">
                    help is here
                  </a>
                  .
                </p>
              )}
            </div>
          </div>
        </div>

        {/* Today's affirmation: a cream card floating over the photo, lower
            right, inside the container's right edge. Always rendered regardless
            of Scripture toggle. */}
        <aside className={styles.heroCard} aria-label="Today's affirmation">
          <p className={styles.heroCardEyebrow}>Today&rsquo;s affirmation</p>
          <p className={styles.heroCardVerse}>
            &ldquo;{cardVerse.text}&rdquo;
          </p>
          <p className={styles.heroCardRef}>
            {cardVerse.reference} ({cardVerse.translation})
          </p>
        </aside>
      </section>

      <div className={styles.heroBand}>
        <div className={styles.inner}>
          <ul className={styles.trustStrip}>
            <li>Anonymous by default</li>
            <li>Faith-friendly, Scripture optional</li>
            <li>Journal, circles and breathing are free</li>
          </ul>
        </div>
      </div>

      {/* ── Features: cards with links ── */}
      <section className={styles.features} id="features">
        <div className={styles.inner}>
          <h2 className={styles.sectionHeading} style={{ textAlign: 'center' }}>What Soulwe gives you</h2>
          <p className={styles.sectionLead} style={{ textAlign: 'center', margin: '0 auto var(--space-6)' }}>
            Four quiet places to look after yourself, at your own pace. You can begin without an account.
          </p>
        </div>
        <div className={styles.featuresBand}>
          <div className={styles.inner}>
            <div className={styles.featuresGrid}>
              {features.map(f => {
                const Icon = f.icon
                return (
                  <div key={f.title} className={styles.featureCard}>
                    <div className={styles.featureIconWrap}>
                      <Icon
                        className={[
                          styles.featureIcon,
                          featureAccentClass[f.accent] ?? '',
                        ].join(' ')}
                        aria-hidden="true"
                      />
                    </div>
                    <h3 className={styles.featureTitle}>{f.title}</h3>
                    <p className={styles.featureDesc}>{f.desc}</p>
                    {f.extraLink && (
                      <Link className={styles.featurePrivacyLink} to={f.extraLink.to}>
                        {f.extraLink.label}
                      </Link>
                    )}
                    {f.link && (
                      <Link className={styles.featureLink} to={f.link.to}>
                        {f.link.label} <ArrowRight aria-hidden="true" size={18} />
                      </Link>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
        <div className={styles.inner}>
          <p className={styles.featureExtra} style={{ textAlign: 'center', margin: 'var(--space-6) auto 0' }}>
            A gentle daily check-in. Say how you are, from heavy to grateful, and look back at how your days have been.
          </p>
        </div>
      </section>

      {/* ── Circles: dark band, centered text, plain topic list ── */}
      <section className={styles.circles} id="circles">
        <div className={styles.inner}>
          <h2 className={styles.sectionHeadingLight}>
            You are not the only one carrying this.
          </h2>
          <p className={styles.circlesIntro}>
            Anonymous groups for grief, work stress, family pressure and doubts
            about faith. You pick a nickname. Nobody sees your name.
          </p>
          <p className={styles.circlesLabel}>Circles include</p>
          <ul className={styles.circlesList}>
            {circleTopics.map(name => (
              <li key={name}>{name}</li>
            ))}
          </ul>
          <Link className={styles.btnSun} to="/circle">
            Join a circle anonymously
          </Link>
        </div>
      </section>

      {/* ── Therapists: text left, real directory or honest placeholder right ── */}
      <section className={styles.therapists} id="therapists">
        <div className={styles.inner}>
          <div className={styles.therapistsGrid}>
            <div className={styles.therapistsText}>
              <h2 className={styles.sectionHeading}>
                Therapists who speak your language
              </h2>
              <p className={styles.sectionLead}>
                Sessions in Swahili, Dholuo, Kikuyu and English, from KES 500.
                You choose who you talk to and when.
              </p>
              <Link className={styles.btnPrimary} to="/therapist">
                Find your therapist
              </Link>
            </div>

            <div className={styles.therapistsPanel}>
              {therapists.length > 0 ? (
                <ul className={styles.therapistList}>
                  {therapists.slice(0, 3).map(t => (
                    <li key={t.id} className={styles.therapistRow}>
                      <div className={styles.therapistInfo}>
                        <p className={styles.therapistName}>{t.display_name}</p>
                        <p className={styles.therapistLang}>
                          {t.languages.join(' · ')}
                        </p>
                        {t.specialties.length > 0 && (
                          <p className={styles.therapistMeta}>
                            {t.specialties.join(' · ')}
                          </p>
                        )}
                      </div>
                      <span className={styles.therapistPrice}>
                        {formatPrice(t)}
                      </span>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className={styles.therapistPlaceholder}>
                  <h3>Therapists are joining.</h3>
                  <p>
                    The directory is still filling up. Browse it to see who is
                    open for sessions right now.
                  </p>
                  <Link className={styles.textLink} to="/therapist">
                    See the therapist directory
                  </Link>
                </div>
              )}
            </div>
          </div>
        </div>
      </section>

      {/* ── Stats: three honest, checkable facts ── */}
      <div className={styles.stats}>
        <div className={styles.inner}>
          <dl className={styles.statsList}>
            <div className={styles.statItem}>
              <dt className={styles.statLabel}>Journal, circles and breathing</dt>
              <dd className={styles.statNum}>Free</dd>
            </div>
            <div className={styles.statItem}>
              <dt className={styles.statLabel}>Therapist sessions from</dt>
              <dd className={styles.statNum}>KES 500</dd>
            </div>
            <div className={styles.statItem}>
              <dt className={styles.statLabel}>Kenyan therapy languages</dt>
              <dd className={styles.statNum}>3</dd>
            </div>
          </dl>
        </div>
      </div>

      {/* ── Why Soulwe exists: figures left-list, closing line right ── */}
      <section className={styles.problem}>
        <div className={styles.inner}>
          <div className={styles.problemGrid}>
            <div className={styles.problemText}>
              <h2 className={styles.sectionHeading}>
                Mental health care in East Africa has a gap.
              </h2>
              <p className={styles.problemClose}>
                Soulwe is not a substitute for psychiatric care. It is support
                for the long wait before you can get it.
              </p>
            </div>
            <div className={styles.problemFigures}>
              <div className={styles.figure}>
                <span className={styles.figureNum}>90%</span>
                <p className={styles.figureText}>
                  of people who need mental health support in sub-Saharan Africa
                  never receive it.
                  {statsSourceUrl !== '' && (
                    <sup className={styles.footnoteRef}>1</sup>
                  )}
                </p>
              </div>
              <div className={styles.figure}>
                <span className={styles.figureNum}>&lt;100</span>
                <p className={styles.figureText}>
                  psychiatrists serve the whole of Kenya.
                  {statsSourceUrl !== '' && (
                    <sup className={styles.footnoteRef}>1</sup>
                  )}
                </p>
              </div>
              {statsSourceUrl !== '' && (
                <p className={styles.footnote}>
                  <sup>1</sup>{' '}
                  <a
                    className={styles.footnoteLink}
                    href={statsSourceUrl}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Source for both figures
                  </a>
                </p>
              )}
            </div>
          </div>
        </div>
      </section>

      {/* ── Final CTA: left-aligned, no glow ── */}
      <section className={styles.finalCta}>
        <div className={styles.inner}>
          <h2 className={styles.finalCtaHeading}>Start whenever you&rsquo;re ready.</h2>
          <p className={styles.finalCtaSub}>
            No sign-up required to look around. Walk in anonymously, stay as
            long as you need.
          </p>
          <Link className={styles.btnPrimary} to="/home">
            Enter Soulwe
          </Link>
        </div>
      </section>

    </div>
  )
}
