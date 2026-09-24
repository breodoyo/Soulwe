import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { isApiError } from '@/types'
import styles from './AuthForm.module.css'

export default function RegisterPage() {
  const { register } = useAuth()
  const navigate = useNavigate()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (submitting) return
    setError(null)

    if (!email.trim()) {
      setError('Please enter your email address.')
      return
    }
    if (password.length < 12) {
      setError('Your password must be at least 12 characters long.')
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }

    try {
      setSubmitting(true)
      await register(email.trim(), password)
      navigate('/login', { replace: true, state: { registered: true } })
    } catch (err) {
      setError(
        isApiError(err)
          ? err.status === 409
            ? 'An account with this email already exists. Try signing in instead.'
            : err.message
          : 'We could not create your account right now. Please try again.',
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
          <h1 className={styles.heading}>Find your space</h1>
          <p className={styles.sub}>Create an account to start your journey.</p>
        </div>

        {error && <p className={styles.error} role="alert">{error}</p>}

        <form className={styles.form} onSubmit={handleSubmit} noValidate>
          <div className={styles.field}>
            <label className={styles.label} htmlFor="register-email">Email</label>
            <input
              id="register-email"
              className={styles.input}
              type="email"
              autoComplete="email"
              placeholder="you@example.com"
              value={email}
              onChange={e => setEmail(e.target.value)}
            />
          </div>

          <div className={styles.field}>
            <label className={styles.label} htmlFor="register-password">Password</label>
            <input
              id="register-password"
              className={styles.input}
              type="password"
              autoComplete="new-password"
              placeholder="At least 12 characters"
              value={password}
              onChange={e => setPassword(e.target.value)}
            />
            <p className={styles.hint}>Use 12 or more characters — a phrase is fine.</p>
          </div>

          <div className={styles.field}>
            <label className={styles.label} htmlFor="register-confirm">Confirm password</label>
            <input
              id="register-confirm"
              className={styles.input}
              type="password"
              autoComplete="new-password"
              placeholder="Repeat your password"
              value={confirm}
              onChange={e => setConfirm(e.target.value)}
            />
          </div>

          <button className={styles.submit} type="submit" disabled={submitting}>
            {submitting ? 'Creating your account…' : 'Create account'}
          </button>
        </form>

        <p className={styles.foot}>
          Already have an account? <Link to="/login">Sign in</Link>
        </p>
        <p className={styles.backLink}><Link to="/">← Back to the Soulwe home page</Link></p>

      </div>
    </main>
  )
}