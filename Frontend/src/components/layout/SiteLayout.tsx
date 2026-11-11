import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import { LifeBuoy } from 'lucide-react'
import { useAuth } from '@/auth/AuthContext'
import { crisisHelplines } from '@/landingContent'
import styles from './SiteLayout.module.css'

interface NavItem {
  to: string
  label: string
  end?: boolean
}

const navLinks: NavItem[] = [
  { to: '/',          label: 'Home',      end: true },
  { to: '/#features', label: 'Features'  },
  { to: '/circle',    label: 'Circles'   },
  { to: '/therapist', label: 'Therapists'},
  { to: '/journal',   label: 'Journal'   },
  { to: '/breathe',   label: 'Breathe'   },
]

const crisisLink: NavItem = { to: '#crisis', label: 'Crisis help' }

const footerExploreLinks: NavItem[] = [
  { to: '/journal',   label: 'Journal'    },
  { to: '/circle',    label: 'Circles'    },
  { to: '/breathe',   label: 'Breathe'    },
  { to: '/therapist', label: 'Therapists' },
]

const footerSiteLinks: NavItem[] = [
  { to: '/privacy', label: 'Privacy' },
  { to: '/terms',   label: 'Terms'   },
  { to: '/contact', label: 'Contact' },
  crisisLink,
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

  useEffect(() => {
    setMenuOpen(false)
  }, [pathname, hash])

  // Cross-page anchors need scrolling after the target page mounts.
  useEffect(() => {
    if (!hash) return
    const id = decodeURIComponent(hash.slice(1))
    const el = document.getElementById(id)
    if (el) el.scrollIntoView()
  }, [pathname, hash])

  // Move focus to the main landmark after route changes, except on anchor jumps.
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

      {/* tabIndex -1 allows programmatic focus without adding a Tab stop. */}
      <main className={styles.main} id="main-content" tabIndex={-1}>
        <Outlet />
      </main>

      {/* Plain, always visible; never publish a helpline from memory. */}
      <section className={styles.crisis} id="crisis" aria-labelledby="crisis-heading">
        <div className={styles.crisisInner}>
          <div className={styles.crisisHeadingWrap}>
            <span className={styles.crisisIcon} aria-hidden="true">
              <LifeBuoy size={20} strokeWidth={1.8} />
            </span>
            <div>
              <p className={styles.crisisEyebrow}>Crisis support</p>
              <h2 className={styles.crisisHeading} id="crisis-heading">
                If you need help right now
              </h2>
            </div>
          </div>
          <div className={styles.crisisBody}>
            <p className={styles.crisisText}>
              If you or someone near you is in immediate danger, please contact
              your local emergency services or someone you trust. Soulwe is here
              for everyday support, and you don&rsquo;t have to face the hard
              moments alone.
            </p>
            {/* Only real, verified helplines from landingContent.ts render. */}
            {crisisHelplines.length > 0 && (
              <ul className={styles.crisisList}>
                {crisisHelplines.map(line => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </section>

      <footer className={styles.footer} role="contentinfo">
        <div className={styles.footerInner}>
          <div className={styles.footerTop}>
            <div className={styles.footerBrandBlock}>
              <Link to="/" className={styles.footerBrand} aria-label="Soulwe home page">
                <BrandMark />
                <span className={styles.footerName}>Soulwe</span>
              </Link>
              <p className={styles.footerTagline}>
                A calm place to check in with yourself.
              </p>
            </div>

            <nav className={styles.footerColumns} aria-label="Footer">
              <div className={styles.footerColumn}>
                <p className={styles.footerLabel}>Explore</p>
                <ul className={styles.footerList}>
                  {footerExploreLinks.map(item => (
                    <li key={item.to}>
                      <Link className={styles.footerLink} to={item.to}>
                        {item.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
              <div className={styles.footerColumn}>
                <p className={styles.footerLabel}>Information</p>
                <ul className={styles.footerList}>
                  {footerSiteLinks.map(item => (
                    <li key={item.to}>
                      <Link
                        className={`${styles.footerLink}${
                          item.to === '#crisis' ? ` ${styles.footerLinkCrisis}` : ''
                        }`}
                        to={item.to}
                      >
                        {item.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            </nav>
          </div>

          <div className={styles.footerDivider} aria-hidden="true" />

          <div className={styles.footerMeta}>
            <p className={styles.footerCopy}>© 2026 Soulwe.</p>
            {isAuthed ? (
              <p className={styles.sessionLine}>
                Signed in{user?.display_name ? ` as ${user.display_name}` : ''}
              </p>
            ) : (
              <p className={styles.sessionLine}>Browsing anonymously, no account needed.</p>
            )}
          </div>
        </div>
      </footer>

    </div>
  )
}
