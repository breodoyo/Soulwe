// Route guards that gate the app behind authentication while the session is
// being restored, avoiding a flash of protected content.

import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import styles from './RouteGuards.module.css'

function AuthLoader() {
  return (
    <div className={styles.loader} role="status" aria-label="Loading your space">
      <span className={styles.loaderMark} aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
          <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
        </svg>
      </span>
      <p>Opening your space…</p>
    </div>
  )
}

// Wraps pages that require a registered-user session. Unauthenticated visitors
// are sent to /login, remembering where they were headed.
export function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useAuth()
  const location = useLocation()

  if (status === 'loading') return <AuthLoader />
  if (status === 'unauthenticated') {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <>{children}</>
}

// Wraps public pages (login/register) that don't make sense for a signed-in
// user; authenticated visitors are sent back to the app.
export function GuestOnly({ children }: { children: ReactNode }) {
  const { status } = useAuth()

  if (status === 'loading') return <AuthLoader />
  if (status === 'authenticated') return <Navigate to="/home" replace />
  return <>{children}</>
}