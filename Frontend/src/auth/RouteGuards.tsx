// Soulwe is publicly browsable: no route redirects a guest, and authentication is requested per feature via SignInPrompt.

import type { ReactNode } from 'react'
import { Link, Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import styles from './RouteGuards.module.css'

function AuthLoader() {
  return (
    <div className={styles.loader} role="status" aria-label="Loading your space">
      <span className={styles.loaderMark} aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
          <path d="12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
        </svg>
      </span>
      <p>Opening your space…</p>
    </div>
  )
}

// The "this one action needs an account" notice; it never renders on page load and never wraps page content.
export function SignInPrompt({
  title,
  message,
  compact = false,
}: {
  title?: string
  message?: ReactNode
  compact?: boolean
}) {
  const location = useLocation()
  const from = location.pathname

  return (
    <section
      className={compact ? [styles.prompt, styles.promptCompact].join(' ') : styles.prompt}
      aria-labelledby={compact ? undefined : 'signin-prompt-title'}
    >
      <p className={styles.promptEyebrow}>Free account needed</p>
      <h2 className={styles.promptTitle} id={compact ? undefined : 'signin-prompt-title'}>
        {title ?? 'Sign in to use this'}
      </h2>
      <p className={styles.promptBody}>
        {message ??
          'Browsing Soulwe never needs an account. This part keeps your own information, so it needs one.'}
      </p>
      <div className={styles.promptActions}>
        <Link className={styles.promptPrimary} to="/login" state={{ from }}>
          Log in
        </Link>
        <Link className={styles.promptSecondary} to="/register" state={{ from }}>
          Create a free account
        </Link>
      </div>
      <p className={styles.promptNote}>
        You can keep browsing Soulwe without an account.
      </p>
    </section>
  )
}

// Sends signed-in visitors away from pages meant only for guests.
export function GuestOnly({ children }: { children: ReactNode }) {
  const { status } = useAuth()

  if (status === 'loading') return <AuthLoader />
  if (status === 'authenticated') return <Navigate to="/home" replace />
  return <>{children}</>
}
