import { Outlet, NavLink, useLocation, Link } from 'react-router-dom'
import { useEffect, useRef } from 'react'
import { useAuth } from '@/auth/AuthContext'
import styles from './AppShell.module.css'

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
const ProfileIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="8" r="4" />
    <path d="M4 20v-1a7 7 0 0 1 7-7h2a7 7 0 0 1 7 7v1" />
  </svg>
)
const LogoutIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
    <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
    <polyline points="16 17 21 12 16 7" />
    <line x1="21" y1="12" x2="9" y2="12" />
  </svg>
)

const tabs = [
  { to: '/home',      label: 'Home',      swahili: 'Nyumba',  Icon: HomeIcon      },
  { to: '/journal',   label: 'Journal',   swahili: 'Diary',   Icon: JournalIcon   },
  { to: '/circle',    label: 'Circle',    swahili: 'Duara',   Icon: CircleIcon    },
  { to: '/therapist', label: 'Therapist', swahili: 'Mshauri', Icon: TherapistIcon },
  { to: '/breathe',   label: 'Breathe',   swahili: 'Pumzika', Icon: BreatheIcon   },
  { to: '/profile',   label: 'Profile',   swahili: 'Wasifu',  Icon: ProfileIcon   },
]

const breadcrumbLabels: Record<string, string> = {
  '/home':      'Home',
  '/journal':   'Journal',
  '/circle':    'Circle',
  '/therapist': 'Therapist',
  '/breathe':   'Breathe',
  '/profile':   'Profile',
}

function Breadcrumbs() {
  const location = useLocation()
  const label = breadcrumbLabels[location.pathname]
  if (!label || location.pathname === '/home') return null

  return (
    <nav className={styles.breadcrumbs} aria-label="Breadcrumb">
      <Link to="/home" className={styles.breadcrumbHome}>
        Home
      </Link>
      <span className={styles.breadcrumbSep} aria-hidden="true">/</span>
      <span className={styles.breadcrumbCurrent} aria-current="page">
        {label}
      </span>
    </nav>
  )
}

// The session badge is the app's answer to "am I anonymous right now?": a
// guest always sees "Anonymous session" and a way in, a registered user sees
// their own name and a way out. It is driven by the real auth status, so it can
// never claim a signed-in visitor is browsing anonymously.
function SessionBadge() {
  const { status, user } = useAuth()

  if (status === 'authenticated') {
    return (
      <span className={styles.memberBadge}>
        <span className={styles.memberDot} aria-hidden="true" />
        {user?.display_name ? `Signed in as ${user.display_name}` : 'Signed in'}
      </span>
    )
  }

  return (
    <span className={styles.anonBadge} aria-label="You are browsing anonymously">
      <span className={styles.anonDot} aria-hidden="true" />
      Anonymous session
    </span>
  )
}

export default function AppShell() {
  const { status, logout } = useAuth()
  const isGuest = status !== 'authenticated'
  const { pathname } = useLocation()
  const firstRender = useRef(true)

  // Route changes in a single-page app leave focus on the link that was
  // activated, so assistive tech still believes the user is on the previous
  // page. Move focus to the main landmark after each navigation so the next
  // Tab starts from the new content. Skipped on mount, which must not steal
  // focus from the browser or a deep link.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    const main = document.getElementById('main-content')
    if (main instanceof HTMLElement) main.focus()
  }, [pathname])

  return (
    <div className={styles.shell}>

      {/* First tab stop: lets keyboard users bypass the nav, session badge and
          breadcrumbs. Hidden until focused, then pinned to the top. */}
      <a className={styles.skipLink} href="#main-content">Skip to main content</a>

      {/* Top nav — mobile only */}
      <header className={styles.topnav} role="banner">
        <Link to="/" className={styles.brand} aria-label="Go to Soulwe home page">
          <div className={styles.brandMark} aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
            </svg>
          </div>
          <span className={styles.brandName}>Soulwe</span>
        </Link>
        <div className={styles.topnavActions}>
          <SessionBadge />
          {isGuest ? (
            <>
              <Link className={styles.authLink} to="/login">Log in</Link>
              <Link className={styles.authLink} to="/register">Register</Link>
            </>
          ) : (
            <button className={styles.logoutBtn} onClick={logout} aria-label="Log out">
              <LogoutIcon />
              <span>Log out</span>
            </button>
          )}
        </div>
      </header>

      {/* Breadcrumbs */}
      <Breadcrumbs />

      {/* Page content. tabIndex -1 so route changes can move focus here without
          making the landmark reachable by Tab. */}
      <main className={styles.content} id="main-content" tabIndex={-1}>
        <Outlet />
      </main>

      {/* Tab bar — mobile bottom / desktop left sidebar */}
      <nav className={styles.tabbar} aria-label="Main navigation">

        {/* Desktop sidebar brand — real link, not CSS pseudo-element */}
        <Link to="/" className={styles.sidebarBrand} aria-label="Go to Soulwe landing page">
          <div className={styles.brandMark} aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
            </svg>
          </div>
          <span className={styles.brandName}>Soulwe</span>
        </Link>

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

        {/* Desktop sidebar session state — real, not a CSS pseudo-element */}
        <div className={styles.sidebarSession}>
          <SessionBadge />
          {isGuest ? (
            <div className={styles.sidebarAuthLinks}>
              <Link className={styles.sidebarAuthLink} to="/login">Log in</Link>
              <Link className={styles.sidebarAuthLink} to="/register">Register</Link>
            </div>
          ) : null}
        </div>

        {/* Desktop sidebar logout */}
        {!isGuest && (
          <button className={styles.sidebarLogout} onClick={logout} aria-label="Log out">
            <span className={styles.tabIcon} aria-hidden="true">
              <LogoutIcon />
            </span>
            <span className={styles.tabLabel}>Log out</span>
          </button>
        )}

      </nav>

    </div>
  )
}