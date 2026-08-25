import { Outlet, NavLink, useLocation } from 'react-router-dom'
import styles from './AppShell.module.css'

// Icons — inline SVGs so we have zero icon-library dependency
const HomeIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    <polyline points="9 22 9 12 15 12 15 22" />
  </svg>
)
const JournalIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
    <polyline points="14 2 14 8 20 8" />
    <line x1="16" y1="13" x2="8" y2="13" />
    <line x1="16" y1="17" x2="8" y2="17" />
  </svg>
)
const CircleIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
    <circle cx="9" cy="7" r="4" />
    <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
    <path d="M16 3.13a4 4 0 0 1 0 7.75" />
  </svg>
)
const TherapistIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
    <circle cx="12" cy="7" r="4" />
  </svg>
)
const BreatheIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="10" />
    <path d="M8 14s1.5 2 4 2 4-2 4-2" />
    <line x1="9" y1="9" x2="9.01" y2="9" />
    <line x1="15" y1="9" x2="15.01" y2="9" />
  </svg>
)

const tabs = [
  { to: '/home',      label: 'Home',      swahili: 'Nyumba',  Icon: HomeIcon      },
  { to: '/journal',   label: 'Journal',   swahili: 'Diary',   Icon: JournalIcon   },
  { to: '/circle',    label: 'Circle',    swahili: 'Duara',   Icon: CircleIcon    },
  { to: '/therapist', label: 'Therapist', swahili: 'Mshauri', Icon: TherapistIcon },
  { to: '/breathe',   label: 'Breathe',   swahili: 'Pumzika', Icon: BreatheIcon   },
]

export default function AppShell() {
  return (
    <div className={styles.shell}>

      {/* Top nav */}
      <header className={styles.topnav} role="banner">
        <div className={styles.brand}>
          <div className={styles.brandMark} aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
            </svg>
          </div>
          <span className={styles.brandName}>Soulwe</span>
        </div>
        <span className={styles.anonBadge} aria-label="You are browsing anonymously">
          <span className={styles.anonDot} aria-hidden="true" />
          Anonymous
        </span>
      </header>

      {/* Page content */}
      <main className={styles.content} id="main-content">
        <Outlet />
      </main>

      {/* Bottom tab bar */}
      <nav className={styles.tabbar} aria-label="Main navigation">
        {tabs.map(({ to, label, swahili, Icon }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              [styles.tab, isActive ? styles.tabActive : ''].join(' ')
            }
            aria-label={`${label} — ${swahili}`}
          >
            <span className={styles.tabIcon} aria-hidden="true">
              <Icon />
            </span>
            <span className={styles.tabLabel}>{label}</span>
            <span className={styles.tabSwahili}>{swahili}</span>
          </NavLink>
        ))}
      </nav>

    </div>
  )
}
