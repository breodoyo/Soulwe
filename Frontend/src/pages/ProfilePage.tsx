import { useEffect, useState, type FormEvent } from 'react'
import { api } from '@/lib/api'
import { isApiError, type LanguagePref, type User } from '@/types'
import styles from './ProfilePage.module.css'

const languageOptions: { value: LanguagePref; label: string }[] = [
  { value: 'en',  label: 'English' },
  { value: 'sw',  label: 'Kiswahili' },
  { value: 'luo', label: 'Dholuo (Luo)' },
  { value: 'kik', label: 'Kikuyu' },
]

function isLanguagePref(value: string): value is LanguagePref {
  return value === 'en' || value === 'sw' || value === 'luo' || value === 'kik'
}

function formatMemberSince(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
}

export default function ProfilePage() {
  const [profile, setProfile] = useState<User | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [languagePref, setLanguagePref] = useState<LanguagePref>('en')
  const [loading, setLoading] = useState(true)
  const [feedError, setFeedError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [savedMsg, setSavedMsg] = useState<string | null>(null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setFeedError(null)
    api.users
      .me()
      .then(({ user }) => {
        if (cancelled) return
        setProfile(user)
        setDisplayName(user.display_name ?? '')
        setLanguagePref(isLanguagePref(user.language_pref) ? user.language_pref : 'en')
        setLoading(false)
      })
      .catch(err => {
        if (cancelled) return
        setFeedError(
          isApiError(err)
            ? err.message
            : 'We could not load your profile right now. Please try again.',
        )
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [tick])

  const baseName = profile?.display_name ?? ''
  const baseLang: LanguagePref =
    profile && isLanguagePref(profile.language_pref) ? profile.language_pref : 'en'
  const dirty = profile != null && (displayName.trim() !== baseName || languagePref !== baseLang)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (saving || !dirty || !profile) return
    setSaveError(null)
    setSavedMsg(null)
    setSaving(true)
    try {
      const { user } = await api.users.updateProfile({
        display_name: displayName.trim(),
        language_pref: languagePref,
      })
      setProfile(user)
      setDisplayName(user.display_name ?? '')
      setSavedMsg('Profile saved.')
    } catch (err) {
      setSaveError(
        isApiError(err)
          ? err.message
          : 'We could not save your profile right now. Please try again.',
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className={styles.page}>
      <div>
        <h1 className={styles.heading}>Wasifu wa kwangu</h1>
        <p className={styles.sub}>Your profile — how Soulwe knows you.</p>
      </div>

      {feedError && !profile ? (
        <section className={styles.errorPanel} role="alert">
          <p className={styles.errorEyebrow}>Something went wrong</p>
          <p className={styles.errorText}>{feedError}</p>
          <button className={styles.retryBtn} onClick={() => setTick(t => t + 1)}>
            Try again
          </button>
        </section>
      ) : profile ? (
        <div className={styles.card}>
          {/* Read-only account info */}
          <dl className={styles.infoList}>
            <div className={styles.infoRow}>
              <dt>Email</dt>
              <dd className={styles.infoValue}>{profile.email}</dd>
            </div>
            <div className={styles.infoRow}>
              <dt>Account</dt>
              <dd>{profile.is_verified ? 'Verified' : 'Unverified'}</dd>
            </div>
            <div className={styles.infoRow}>
              <dt>Member since</dt>
              <dd>{formatMemberSince(profile.created_at)}</dd>
            </div>
          </dl>

          <form className={styles.form} onSubmit={handleSubmit} noValidate>
            <div className={styles.field}>
              <label className={styles.label} htmlFor="profile-name">Display name</label>
              <input
                id="profile-name"
                className={styles.input}
                type="text"
                maxLength={100}
                placeholder="What should we call you?"
                value={displayName}
                onChange={e => setDisplayName(e.target.value)}
              />
              <p className={styles.hint}>Leave blank to stay anonymous.</p>
            </div>

            <div className={styles.field}>
              <label className={styles.label} htmlFor="profile-language">Language</label>
              <select
                id="profile-language"
                className={styles.select}
                value={languagePref}
                onChange={e => setLanguagePref(e.target.value as LanguagePref)}
              >
                {languageOptions.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
              <p className={styles.hint}>The language we speak to you in.</p>
            </div>

            {saveError && <p className={styles.error} role="alert">{saveError}</p>}
            {savedMsg && <p className={styles.success} role="status">{savedMsg}</p>}

            <button className={styles.submit} type="submit" disabled={!dirty || saving}>
              {saving ? 'Saving…' : 'Save changes'}
            </button>
          </form>
        </div>
      ) : (
        <p className={styles.loading}>
          {loading ? 'Loading your profile…' : 'Profile unavailable.'}
        </p>
      )}
    </div>
  )
}