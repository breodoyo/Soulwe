import { useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { isApiError } from '@/types'
import styles from './AuthForm.module.css'

interface LoginLocationState {
  from?: string
  registered?: boolean
}

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const state = location.state as LoginLocationState | null
  const from = state?.from ?? '/home'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (submitting) return
    setError(null)

    if (!email.trim() || !password) {
      setError('Please enter your email and password.')
      return
    }

    try {
      setSubmitting(true)
      await login(email.trim(), password)
      navigate(from, { replace: true })
    } catch (err) {
      setError(
        isApiError(err)
          ? err.status === 401
            ? 'Incorrect email or password. Please try again.'
            : err.message
          : 'We could not sign you in right now. Please try again.',
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className={styles.page}>
      <div className={styles.card}>

        <div className={styles.brand}>
          <div className={styles.brandMark} aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <path d="M12 21C12 21 4 13.5 4 8.5a5 5 0 0 1 8-4 5 5 0 0 1 8 4c0 5-8 12.5-8 12.5z" />
            </svg>
          </div>
          <span className={styles.brandName}>Soulwe</span>
        </div>

        <div>
          <h1 className={styles.heading}>Karibu tena</h1>
          <p className={styles.sub}>Sign in to continue your journey.</p>
        </div>

        {state?.registered && (
          <p className={styles.success} role="status">
            Your account was created. Welcome to Soulwe — please sign in.
          </p>
        )}
        {error && <p className={styles.error} role="alert">{error}</p>}

        <form className={styles.form} onSubmit={handleSubmit} noValidate>
          <div className={styles.field}>
            <label className={styles.label} htmlFor="login-email">Email</label>
            <input
              id="login-email"
              className={styles.input}
              type="email"
              autoComplete="email"
              placeholder="you@example.com"
              value={email}
              onChange={e => setEmail(e.target.value)}
            />
          </div>

          <div className={styles.field}>
            <label className={styles.label} htmlFor="login-password">Password</label>
            <input
              id="login-password"
              className={styles.input}
              type="password"
              autoComplete="current-password"
              placeholder="Your password"
              value={password}
              onChange={e => setPassword(e.target.value)}
            />
          </div>

          <button className={styles.submit} type="submit" disabled={submitting}>
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        <p className={styles.foot}>
          New to Soulwe? <Link to="/register">Create an account</Link>
        </p>
        <p className={styles.backLink}><Link to="/">← Back to the Soulwe home page</Link></p>

      </div>
    </main>
  )
}