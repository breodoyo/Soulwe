import { useState } from 'react'
import styles from './TherapistPage.module.css'

interface Therapist {
  id: string; name: string; initials: string; color: string
  credentials: string; location: string; priceKes: number
  freeSessions: number; specialties: string[]; languages: string[]
  nextSlot: string; isOnline: boolean
}

const therapists: Therapist[] = [
  { id:'1', name:'Dr. Amina Korir', initials:'AK', color:'#C4714A',
    credentials:'PhD Clinical Psychology · Kenyatta University · 8 yrs',
    location:'Nairobi / Online', priceKes:800, freeSessions:2,
    specialties:['Grief','Trauma','Family'], languages:['Swahili','English'],
    nextSlot:'Today 4:00 PM', isOnline:true },
  { id:'2', name:'Joel Odhiambo', initials:'JO', color:'#4A6741',
    credentials:'MA Counselling Psychology · UoN · 5 yrs',
    location:'Kisumu / Online', priceKes:500, freeSessions:1,
    specialties:['Anxiety','Youth','Relationships'], languages:['Dholuo','Swahili'],
    nextSlot:'Thu 10:00 AM', isOnline:false },
  { id:'3', name:'Wanjiku Waweru', initials:'WW', color:'#5C3D2E',
    credentials:'BSc Psychology, CBT Certified · Moi University · 3 yrs',
    location:'Online only', priceKes:500, freeSessions:2,
    specialties:['Depression','Self-esteem','Women'], languages:['Kikuyu','Swahili'],
    nextSlot:'Tomorrow 9:00 AM', isOnline:true },
]

const FILTERS = ['All','Swahili','Dholuo','Kikuyu','Free sessions','Online only']

export default function TherapistPage() {
  const [activeFilter, setActiveFilter] = useState('All')
  const [booked, setBooked] = useState<string | null>(null)

  const filtered = therapists.filter(t => {
    if (activeFilter === 'All') return true
    if (activeFilter === 'Free sessions') return t.freeSessions > 0
    if (activeFilter === 'Online only') return t.location.includes('Online')
    return t.languages.includes(activeFilter)
  })

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
            className={[styles.filterPill, activeFilter === f ? styles.filterPillActive : ''].join(' ')}
            onClick={() => setActiveFilter(f)}
            aria-pressed={activeFilter === f}
          >
            {f}
          </button>
        ))}
      </div>

      {/* Cards */}
      <div className={styles.cardList}>
        {filtered.map(t => (
          <div key={t.id} className={styles.card}>
            <div className={styles.cardTop}>
              <div className={styles.avatar} style={{ background: t.color }}>
                {t.initials}
              </div>
              <div className={styles.info}>
                <h2 className={styles.name}>{t.name}</h2>
                <p className={styles.creds}>{t.credentials}</p>
                <p className={styles.location}>
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>
                  {t.location}
                </p>
              </div>
            </div>

            {/* Tags */}
            <div className={styles.tags}>
              {t.specialties.map(s => <span key={s} className={styles.tag}>{s}</span>)}
              {t.languages.map(l => <span key={l} className={[styles.tag, styles.tagLang].join(' ')}>{l}</span>)}
            </div>

            {/* Pricing row */}
            <div className={styles.priceRow}>
              <span className={styles.price}>KES {t.priceKes.toLocaleString()}/session</span>
              {t.freeSessions > 0 && (
                <span className={styles.freeTag}>{t.freeSessions} free session{t.freeSessions > 1 ? 's' : ''}</span>
              )}
            </div>

            {/* Availability + book */}
            <div className={styles.cardBottom}>
              <p className={styles.avail}>
                {t.isOnline && <span className={styles.onlineDot} aria-hidden="true" />}
                Next: {t.nextSlot}
              </p>
              <button
                className={[styles.bookBtn, booked === t.id ? styles.bookBtnBooked : ''].join(' ')}
                onClick={() => setBooked(t.id)}
                aria-label={`Book session with ${t.name}`}
              >
                {booked === t.id ? '✓ Requested' : 'Book session'}
              </button>
            </div>

            {booked === t.id && (
              <p className={styles.bookConfirm}>
                {t.name.split(' ')[0]} will confirm within 24 hours. Check your messages.
              </p>
            )}
          </div>
        ))}

        {filtered.length === 0 && (
          <div className={styles.empty}>
            <p>No therapists match this filter yet.</p>
            <button className={styles.clearFilter} onClick={() => setActiveFilter('All')}>Show all therapists</button>
          </div>
        )}
      </div>
    </div>
  )
}
