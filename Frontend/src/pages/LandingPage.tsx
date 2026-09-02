import { useNavigate } from 'react-router-dom'
import styles from './LandingPage.module.css'

const features = [
  {
    icon: '✍️',
    title: 'Journal your truth',
    desc: 'Write freely or pray intentionally. Your entries are private, encrypted, and yours alone. An AI companion responds with warmth rooted in African wisdom and Scripture.',
  },
  {
    icon: '🌿',
    title: 'Find your circle',
    desc: 'Anonymous peer support groups built around real African experiences — grief, family pressure, PTSD, relationships, faith. No names. No judgment. Just people who understand.',
  },
  {
    icon: '🤝',
    title: 'Talk to a therapist',
    desc: 'Kenyan therapists who speak Swahili, Dholuo, and Kikuyu. From KES 500 per session. First sessions subsidised. No waiting lists, no referral letters.',
  },
  {
    icon: '🫁',
    title: 'Breathe through it',
    desc: 'Guided 4-7-8 and box breathing exercises with a calming visual. Two minutes is enough to shift your nervous system from panic to peace.',
  },
]

const stats = [
  { num: '8',    label: 'Support circles'         },
  { num: '3',    label: 'Kenyan languages'         },
  { num: 'Free', label: 'For everyone, always'     },
  { num: '24/7', label: 'Always open'              },
]

const testimonials = [
  {
    text: "I had never told anyone I was struggling. I typed it into a circle at 2am and someone replied within minutes. That was the first time I felt less alone in years.",
    name: 'Anon Baobab',
    location: 'Nairobi',
  },
  {
    text: "The prayer journal changed my mornings. Having a structured space to bring my fears to God — and then an AI that responds with Scripture — it feels like being heard twice.",
    name: 'Anon Willow',
    location: 'Kisumu',
  },
  {
    text: "I booked a therapist who speaks Dholuo. For the first time I could describe my pain in the language I dream in. That matters more than I can explain.",
    name: 'Anon Savanna',
    location: 'Eldoret',
  },
]

const verses = [
  { text: '"Come to me, all you who are weary and burdened, and I will give you rest."', ref: 'Matthew 11:28' },
  { text: '"The Lord is close to the brokenhearted and saves those who are crushed in spirit."', ref: 'Psalm 34:18' },
  { text: '"Cast all your anxiety on him because he cares for you."', ref: '1 Peter 5:7' },
]

export default function LandingPage() {
  const navigate = useNavigate()

  return (
    <div className={styles.page}>

      {/* ── Nav ── */}
      <nav className={styles.nav}>
        <div className={styles.navBrand}>
          <div className={styles.navMark}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z"/>
            </svg>
          </div>
          <span className={styles.navName}>Soulwe</span>
        </div>
        <div className={styles.navLinks}>
          <a href="#features" className={styles.navLink}>Features</a>
          <a href="#circles" className={styles.navLink}>Circles</a>
          <a href="#therapists" className={styles.navLink}>Therapists</a>
        </div>
        <button className={styles.navCta} onClick={() => navigate('/home')}>
          Open app →
        </button>
      </nav>

      {/* ── Hero ── */}
      <section className={styles.hero}>
        <div className={styles.heroInner}>
          <span className={styles.heroPill}>🌍 Built for East Africa</span>
          <h1 className={styles.heroHeading}>
            Your soul deserves<br />
            <em>a place to rest.</em>
          </h1>
          <p className={styles.heroSub}>
            Soulwe is a mental health companion designed around African life —
            the family pressure, the grief you perform, the faith you hold,
            the things you carry alone. You don't have to carry them alone anymore.
          </p>
          <div className={styles.heroCtas}>
            <button className={styles.heroCtaPrimary} onClick={() => navigate('/home')}>
              Enter Soulwe — it's free
            </button>
            <a href="#features" className={styles.heroCtaSecondary}>
              See how it works ↓
            </a>
          </div>
          <p className={styles.heroNote}>
            Anonymous by default. No credit card. No judgment.
          </p>
        </div>

        {/* Decorative verse */}
        <div className={styles.heroVerse}>
          <p className={styles.heroVerseText}>
            "Come to me, all you who are weary and burdened,<br />
            and I will give you rest."
          </p>
          <p className={styles.heroVerseRef}>Matthew 11:28</p>
        </div>
      </section>

      {/* ── Stats ── */}
      <section className={styles.statsBar}>
        {stats.map(s => (
          <div key={s.label} className={styles.statItem}>
            <span className={styles.statNum}>{s.num}</span>
            <span className={styles.statLabel}>{s.label}</span>
          </div>
        ))}
      </section>

      {/* ── The problem ── */}
      <section className={styles.problem}>
        <div className={styles.sectionInner}>
          <span className={styles.eyebrow}>Why Soulwe exists</span>
          <h2 className={styles.sectionHeading}>
            Mental health care in East Africa has a gap.
          </h2>
          <div className={styles.problemGrid}>
            <div className={styles.problemCard}>
              <span className={styles.problemNum}>90%</span>
              <p className={styles.problemText}>
                of people who need mental health support in sub-Saharan Africa never receive it.
              </p>
            </div>
            <div className={styles.problemCard}>
              <span className={styles.problemNum}>&lt;100</span>
              <p className={styles.problemText}>
                psychiatrists serve Kenya's 55 million people. The wait is months. The cost is thousands.
              </p>
            </div>
            <div className={styles.problemCard}>
              <span className={styles.problemNum}>0</span>
              <p className={styles.problemText}>
                mental health apps are designed specifically around African culture, language, and stigma.
              </p>
            </div>
          </div>
          <p className={styles.problemClose}>
            Soulwe is not trying to replace psychiatry.<br />
            It is trying to close the gap between <strong>nothing</strong> and <strong>something.</strong>
          </p>
        </div>
      </section>

      {/* ── Features ── */}
      <section className={styles.features} id="features">
        <div className={styles.sectionInner}>
          <span className={styles.eyebrow}>What Soulwe gives you</span>
          <h2 className={styles.sectionHeading}>
            Everything in one quiet place.
          </h2>
          <div className={styles.featuresGrid}>
            {features.map(f => (
              <div key={f.title} className={styles.featureCard}>
                <span className={styles.featureIcon}>{f.icon}</span>
                <h3 className={styles.featureTitle}>{f.title}</h3>
                <p className={styles.featureDesc}>{f.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Circles ── */}
      <section className={styles.circlesSection} id="circles">
        <div className={styles.sectionInner}>
          <span className={styles.eyebrow}>Community circles</span>
          <h2 className={styles.sectionHeading}>
            You are not the only one carrying this.
          </h2>
          <p className={styles.sectionSub}>
            Anonymous peer support groups built around real African struggles.
            No names. No profiles. Just honest conversation.
          </p>
          <div className={styles.circlesPill}>
            {[
              { icon:'🕊️', name:'Grief & loss'         },
              { icon:'💼', name:'Work pressure'         },
              { icon:'🌿', name:'Family expectations'   },
              { icon:'💜', name:'Trauma & healing'      },
              { icon:'💍', name:'Relationships'         },
              { icon:'🙏', name:'Faith & doubt'         },
              { icon:'🧠', name:'Anxiety & depression'  },
              { icon:'🌱', name:'Young adults'          },
            ].map(c => (
              <span key={c.name} className={styles.circleBadge}>
                {c.icon} {c.name}
              </span>
            ))}
          </div>
          <button className={styles.circlesBtn} onClick={() => navigate('/circle')}>
            Join a circle anonymously →
          </button>
        </div>
      </section>

      {/* ── Testimonials ── */}
      <section className={styles.testimonials}>
        <div className={styles.sectionInner}>
          <span className={styles.eyebrow}>From the community</span>
          <h2 className={styles.sectionHeading}>
            Real words from real people.
          </h2>
          <div className={styles.testimonialGrid}>
            {testimonials.map(t => (
              <div key={t.name} className={styles.testimonialCard}>
                <p className={styles.testimonialText}>"{t.text}"</p>
                <div className={styles.testimonialAuthor}>
                  <span className={styles.testimonialName}>{t.name}</span>
                  <span className={styles.testimonialLocation}>{t.location}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Therapists ── */}
      <section className={styles.therapistsSection} id="therapists">
        <div className={styles.sectionInner}>
          <span className={styles.eyebrow}>Professional support</span>
          <h2 className={styles.sectionHeading}>
            Therapists who speak your language —<br />
            <em>literally.</em>
          </h2>
          <p className={styles.sectionSub}>
            Every therapist on Soulwe is Kenyan, qualified, and chosen because they
            understand what it means to be you. Sessions in Swahili, Dholuo, Kikuyu,
            and English. Starting from KES 500. First sessions often free.
          </p>
          <button className={styles.therapistsBtn} onClick={() => navigate('/therapist')}>
            Find your therapist →
          </button>
        </div>
      </section>

      {/* ── Verse banner ── */}
      <section className={styles.verseBanner}>
        <div className={styles.sectionInner}>
          {verses.map(v => (
            <div key={v.ref} className={styles.verseItem}>
              <p className={styles.verseText}>{v.text}</p>
              <p className={styles.verseRef}>{v.ref}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ── Final CTA ── */}
      <section className={styles.finalCta}>
        <div className={styles.sectionInner}>
          <div className={styles.finalCtaMark}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z"/>
            </svg>
          </div>
          <h2 className={styles.finalCtaHeading}>
            Your healing is allowed<br />to begin today.
          </h2>
          <p className={styles.finalCtaSub}>
            No sign-up required. Walk in anonymously.<br />
            Stay as long as you need.
          </p>
          <button className={styles.finalCtaBtn} onClick={() => navigate('/home')}>
            Enter Soulwe — it's free
          </button>
        </div>
      </section>

      {/* ── Footer ── */}
      <footer className={styles.footer}>
        <div className={styles.footerInner}>
          <div className={styles.footerBrand}>
            <div className={styles.navMark}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z"/>
              </svg>
            </div>
            <span className={styles.navName}>Soulwe</span>
          </div>
          <p className={styles.footerTagline}>
            A home for your soul. Built in East Africa, for East Africa.
          </p>
          <p className={styles.footerNote}>
            If you are in crisis, please contact{' '}
            <strong>Befrienders Kenya: 0800 723 253</strong> (free, 24/7).
          </p>
        </div>
      </footer>

    </div>
  )
}