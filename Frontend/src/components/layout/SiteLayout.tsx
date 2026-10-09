import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import styles from './SiteLayout.module.css'

interface NavItem {
  to: string
  label: string
  end?: boolean
}

// Website navbar: section anchors scroll the landing page, pages navigate.
const navLinks: NavItem[] = [
  { to: '/',          label: 'Home',      end: true },
  { to: '/#features', label: 'Features'  },
  { to: '/circle',    label: 'Circles'   },
  { to: '/therapist', label: 'Therapists'},
  { to: '/journal',   label: 'Journal'   },
  { to: '/breathe',   label: 'Breathe'   },
]

const footerLinks: NavItem[] = [
  ...navLinks,
  { to: '/home',    label: 'Check in' },
  { to: '/profile', label: 'Profile'  },
]

function BrandMark() {
  return (
    <span className={styles.brandMark} aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
        <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
      </svg>
    </span>
  )
}

function NavItemLink({ item, onNavigate }: { item: NavItem; onNavigate?: () => void }) {
  if (item.to.includes('#')) {
    return (
      <Link className={styles.navLink} to={item.to} onClick={onNavigate}>
        {item.label}
      </Link>
    )
  }
  return (
    <NavLink
      to={item.to}
      end={item.end}
      onClick={onNavigate}
      className={({ isActive }) =>
        isActive ? [styles.navLink, styles.navLinkActive].join(' ') : styles.navLink
      }
    >
      {item.label}
    </NavLink>
  )
}

export default function SiteLayout() {
  const { status, user, logout } = useAuth()
  const { pathname, hash } = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)
  const firstRender = useRef(true)
  const isAuthed = status === 'authenticated'

  // Any navigation dismisses the mobile menu.
  useEffect(() => {
    setMenuOpen(false)
  }, [pathname, hash])

  // Cross-page anchors (Features → /#features) must scroll after the landing
  // page mounts; the browser only handles same-document fragment clicks.
  useEffect(() => {
    if (!hash) return
    const id = decodeURIComponent(hash.slice(1))
    const el = document.getElementById(id)
    if (el) el.scrollIntoView()
  }, [pathname, hash])

  // Route changes leave focus on the activated link, so focus moves to the main
  // landmark — skipped on mount and on anchor jumps, which scroll instead.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    if (hash) return
    const main = document.getElementById('main-content')
    if (main instanceof HTMLElement) main.focus()
  }, [pathname, hash])

  useEffect(() => {
    if (!menuOpen) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [menuOpen])

  return (
    <div className={styles.layout}>

      <a className={styles.skipLink} href="#main-content">Skip to main content</a>

      <header className={styles.header} role="banner">
        <div className={styles.headerInner}>
          <Link to="/" className={styles.brand} aria-label="Soulwe home page">
            <BrandMark />
            <span className={styles.brandName}>Soulwe</span>
          </Link>

          <nav className={styles.nav} aria-label="Main">
            {navLinks.map(item => (
              <NavItemLink key={item.to} item={item} />
            ))}
          </nav>

          <div className={styles.authActions}>
            {isAuthed ? (
              <>
                <NavLink
                  to="/profile"
                  className={({ isActive }) =>
                    isActive ? [styles.navLink, styles.navLinkActive].join(' ') : styles.navLink
                  }
                >
                  Profile
                </NavLink>
                <button className={styles.logoutBtn} onClick={logout}>
                  Log out
                </button>
              </>
            ) : (
              <>
                <Link to="/login" className={styles.loginLink}>Log in</Link>
                <Link to="/register" className={styles.navCta}>Start free</Link>
              </>
            )}
          </div>

          <button
            className={styles.menuBtn}
            onClick={() => setMenuOpen(open => !open)}
            aria-expanded={menuOpen}
            aria-controls="site-menu"
            aria-label={menuOpen ? 'Close menu' : 'Open menu'}
          >
            {menuOpen ? (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" aria-hidden="true">
                <line x1="6" y1="6" x2="18" y2="18" />
                <line x1="18" y1="6" x2="6" y2="18" />
              </svg>
            ) : (
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" aria-hidden="true">
                <line x1="4" y1="7" x2="20" y2="7" />
                <line x1="4" y1="12" x2="20" y2="12" />
                <line x1="4" y1="17" x2="20" y2="17" />
              </svg>
            )}
          </button>
        </div>

        {menuOpen && (
          <div className={styles.mobilePanel} id="site-menu">
            <nav className={styles.mobileNav} aria-label="Main (mobile)">
              {navLinks.map(item => (
                <NavItemLink key={item.to} item={item} onNavigate={() => setMenuOpen(false)} />
              ))}
              {isAuthed && (
                <NavItemLink
                  item={{ to: '/profile', label: 'Profile' }}
                  onNavigate={() => setMenuOpen(false)}
                />
              )}
            </nav>
            <div className={styles.mobileAuth}>
              {isAuthed ? (
                <button className={styles.logoutBtn} onClick={logout}>
                  Log out
                </button>
              ) : (
                <>
                  <Link to="/login" className={styles.loginLink}>Log in</Link>
                  <Link to="/register" className={styles.navCta}>Start free</Link>
                </>
              )}
            </div>
          </div>
        )}
      </header>

      {/* tabIndex -1 lets route changes focus it without making it a Tab stop. */}
      <main className={styles.main} id="main-content" tabIndex={-1}>
        <Outlet />
      </main>

      <footer className={styles.footer} role="contentinfo">
        <div className={styles.footerInner}>
          <Link to="/" className={styles.footerBrand}>
            <BrandMark />
            <span className={styles.footerName}>Soulwe</span>
          </Link>
          <p className={styles.footerTagline}>
            A home for your soul. Built in East Africa, for East Africa.
          </p>
          <nav className={styles.footerNav} aria-label="Footer">
            {footerLinks.map(item => (
              <NavItemLink key={item.to} item={item} />
            ))}
          </nav>
          <div className={styles.footerMeta}>
            <p className={styles.footerCopy}>© 2026 Soulwe.</p>
            {isAuthed ? (
              <p className={styles.sessionLine}>
                Signed in{user?.display_name ? ` as ${user.display_name}` : ''}
              </p>
            ) : (
              <p className={styles.sessionLine}>Browsing anonymously — no account needed.</p>
            )}
          </div>
        </div>
      </footer>

    </div>
  )
}
