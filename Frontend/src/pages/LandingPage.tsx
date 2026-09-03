import React from 'react'
import { useNavigate } from 'react-router-dom'
import styles from './LandingPage.module.css'

const features = [
  {
    icon: '✍️',
    title: 'Journal your truth',
    desc: 'Write freely or pray intentionally. Private, encrypted, AI-reflected with African wisdom and Scripture.',
  },
  {
    icon: '🌿',
    title: 'Find your circle',
    desc: 'Anonymous peer support around real African experiences — grief, PTSD, relationships, faith. No names. No judgment.',
  },
  {
    icon: '🤝',
    title: 'Talk to a therapist',
    desc: 'Kenyan therapists in Swahili, Dholuo, Kikuyu. From KES 500. First sessions subsidised.',
  },
  {
    icon: '🫁',
    title: 'Breathe through it',
    desc: 'Guided breathing exercises. Two minutes to shift from panic to peace.',
  },
]

const testimonials = [
  {
    text: "I typed into a circle at 2am and someone replied within minutes. That was the first time I felt less alone in years.",
    name: 'Anon Baobab',
    location: 'Nairobi',
  },
  {
    text: "The prayer journal changed my mornings. A structured space to bring my fears to God — and an AI that responds with Scripture. It feels like being heard twice.",
    name: 'Anon Willow',
    location: 'Kisumu',
  },
  {
    text: "I booked a therapist who speaks Dholuo. For the first time I described my pain in the language I dream in.",
    name: 'Anon Savanna',
    location: 'Eldoret',
  },
]

const circles = [
  { icon: '🕊️', name: 'Grief & loss'        },
  { icon: '💼', name: 'Work pressure'        },
  { icon: '🌿', name: 'Family expectations'  },
  { icon: '💜', name: 'Trauma & healing'     },
  { icon: '💍', name: 'Relationships'        },
  { icon: '🙏', name: 'Faith & doubt'        },
  { icon: '🧠', name: 'Anxiety & depression' },
  { icon: '🌱', name: 'Young adults'         },
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
          <a href="#features"   className={styles.navLink}>Features</a>
          <a href="#circles"    className={styles.navLink}>Circles</a>
          <a href="#therapists" className={styles.navLink}>Therapists</a>
        </div>
        <button className={styles.navCta} onClick={() => navigate('/home')}>
          Start your journey
        </button>
      </nav>

      {/* ── Hero ── */}
      <section className={styles.hero}>
        <div className={styles.heroLeft}>
          <h1 className={styles.heroHeading}>
            Your soul deserves<br />
            the same care as<br />
            your body.
          </h1>
          <p className={styles.heroSub}>
            Connect with peer support circles, AI-assisted journaling,
            and Kenyan therapists who listen, guide, and walk with you —
            every step of your healing journey.
          </p>
          <button className={styles.heroCtaPrimary} onClick={() => navigate('/home')}>
            Enter Soulwe — it's free
          </button>
          <div className={styles.heroPills}>
            <span className={styles.heroPill}>
              <span className={styles.pillStar}>✦</span> Anonymous by default
            </span>
            <span className={styles.heroPill}>
              <span className={styles.pillStar}>✦</span> Bible-grounded support
            </span>
            <span className={styles.heroPill}>
              <span className={styles.pillStar}>✦</span> Kenyan therapists
            </span>
            <span className={styles.heroPill}>
              <span className={styles.pillStar}>✦</span> Free for everyone
            </span>
          </div>
        </div>

        <div className={styles.heroRight}>
          <div className={styles.heroVisual}>
            <div className={styles.heroOrb} />
            <div className={styles.heroCard}>
              <p className={styles.heroCardEyebrow}>Today's affirmation</p>
              <p className={styles.heroCardVerse}>
                "Come to me, all you who are weary and burdened,
                and I will give you rest."
              </p>
              <p className={styles.heroCardRef}>Matthew 11:28</p>
            </div>
            <div className={styles.heroStat}>
              <span className={styles.heroStatNum}>12</span>
              <span className={styles.heroStatLabel}>people in your circle right now</span>
            </div>
          </div>
        </div>
      </section>

      {/* ── Stats bar ── */}
      <div className={styles.statsBar}>
        {[
          { num: '8',    label: 'Support circles'     },
          { num: '3+',   label: 'Kenyan languages'     },
          { num: 'Free', label: 'Always, for everyone' },
          { num: '24/7', label: 'Always open'          },
        ].map(s => (
          <div key={s.label} className={styles.statItem}>
            <span className={styles.statNum}>{s.num}</span>
            <span className={styles.statLabel}>{s.label}</span>
          </div>
        ))}
      </div>

      {/* ── Features ── */}
      <section className={styles.features} id="features">
        <div className={styles.inner}>
          <span className={styles.eyebrow}>What Soulwe gives you</span>
          <h2 className={styles.sectionHeading}>Everything in one quiet place.</h2>
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

      {/* ── Problem ── */}
      <section className={styles.problem}>
        <div className={styles.inner}>
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
                mental health apps designed specifically around African culture, language, and stigma.
              </p>
            </div>
          </div>
          <p className={styles.problemClose}>
            Soulwe is not trying to replace psychiatry.<br />
            It is trying to close the gap between <strong>nothing</strong> and <strong>something.</strong>
          </p>
        </div>
      </section>

      {/* ── Circles ── */}
      <section className={styles.circlesSection} id="circles">
        <div className={styles.inner}>
          <span className={styles.eyebrowLight}>Community circles</span>
          <h2 className={styles.sectionHeadingLight}>
            You are not the only one carrying this.
          </h2>
          <p className={styles.sectionSubLight}>
            Anonymous peer support groups built around real African struggles.
            No names. No profiles. Just honest conversation.
          </p>
          <div className={styles.circlesPills}>
            {circles.map(c => (
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
        <div className={styles.inner}>
          <span className={styles.eyebrow}>From the community</span>
          <h2 className={styles.sectionHeading}>Real words from real people.</h2>
          <div className={styles.testimonialsGrid}>
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
      <section className={styles.therapists} id="therapists">
        <div className={styles.therapistsInner}>
          <div className={styles.therapistsLeft}>
            <span className={styles.eyebrow}>Professional support</span>
            <h2 className={styles.sectionHeading}>
              Therapists who speak<br />
              your language —<br />
              <em>literally.</em>
            </h2>
            <p className={styles.therapistsDesc}>
              Every therapist on Soulwe is Kenyan, qualified, and chosen because
              they understand what it means to be you. Sessions in Swahili, Dholuo,
              Kikuyu, and English. Starting from KES 500. First sessions often free.
            </p>
            <button className={styles.therapistsBtn} onClick={() => navigate('/therapist')}>
              Find your therapist →
            </button>
          </div>
          <div className={styles.therapistsRight}>
            {[
              { initials:'AK', name:'Dr. Amina Korir', lang:'Swahili · English', spec:'Grief, Trauma',     price:'KES 800' },
              { initials:'JO', name:'Joel Odhiambo',   lang:'Dholuo · Swahili',  spec:'Anxiety, Youth',    price:'KES 500' },
              { initials:'WW', name:'Wanjiku Waweru',   lang:'Kikuyu · Swahili',  spec:'Depression, Women', price:'KES 500' },
            ].map((t, i) => (
              <div
                key={t.name}
                className={styles.therapistMiniCard}
                style={{ '--delay': `${i * 0.1}s` } as React.CSSProperties}
              >
                <div className={styles.therapistMiniAvatar}>{t.initials}</div>
                <div className={styles.therapistMiniInfo}>
                  <p className={styles.therapistMiniName}>{t.name}</p>
                  <p className={styles.therapistMiniLang}>{t.lang}</p>
                  <p className={styles.therapistMiniSpec}>{t.spec}</p>
                </div>
                <span className={styles.therapistMiniPrice}>{t.price}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── Final CTA ── */}
      <section className={styles.finalCta}>
        <div className={styles.finalCtaInner}>
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
          <p className={styles.finalCtaVerse}>
            "The Lord is close to the brokenhearted and saves those who are crushed in spirit." — Psalm 34:18
          </p>
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
            <span className={styles.footerName}>Soulwe</span>
          </div>
          <p className={styles.footerTagline}>
            A home for your soul. Built in East Africa, for East Africa.
          </p>
          <p className={styles.footerCrisis}>
            If you are in crisis — <strong>Befrienders Kenya: 0800 723 253</strong> (free, 24/7)
          </p>
          <p className={styles.footerCopy}>© 2026 Soulwe. Made with care in Kisumu, Kenya.</p>
        </div>
      </footer>

    </div>
  )
}