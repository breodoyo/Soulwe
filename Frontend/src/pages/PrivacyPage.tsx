import { Link } from 'react-router-dom'
import styles from './LegalPages.module.css'

// Stub. Honest skeleton only: no encryption or data-handling claim is made
// here that the backend has not confirmed.
export default function PrivacyPage() {
  return (
    <div className={styles.page}>
      <div className={styles.inner}>
        <h1 className={styles.heading}>Privacy</h1>
        <p className={styles.meta}>Last updated: TODO(real data: launch date)</p>

        <div className={styles.body}>
          <h2>Your journal</h2>
          <p>
            Your journal is private to you. Nobody else can read your entries.
          </p>
          <p>
            Optional AI reflections send the entry you choose to an AI provider.
            You can skip reflections and keep an entry entirely to yourself.
          </p>
          <p className={styles.todo}>
            TODO(real data): name the AI provider(s), state what is sent, how
            long it is retained, and whether it is used for training. Do not
            publish this page until that is written and reviewed.
          </p>

          <h2>Circles</h2>
          <p>
            Circles are anonymous. You get a different anonymous name in each circle, and other members see
            that name, not your real name or account.
          </p>
          <p className={styles.todo}>
            TODO(real data): state exactly what circle activity is stored, for
            how long, and how reporting and moderation work.
          </p>

          <h2>Accounts</h2>
          <p className={styles.todo}>
            TODO(real data): what account data is collected (email, language
            preference), where it is hosted, and how a user deletes it.
          </p>

          <h2>Contact</h2>
          <p>
            Questions about your data: <Link to="/contact">get in touch</Link>.
          </p>
        </div>

        <Link className={styles.back} to="/">
          Back to the home page
        </Link>
      </div>
    </div>
  )
}
