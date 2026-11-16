import { Link } from 'react-router-dom'
import styles from './LegalPages.module.css'

// Stub: no data-handling claim the backend has not confirmed.
export default function PrivacyPage() {
  return (
    <div className={styles.page}>
      <div className={styles.inner}>
        <h1 className={styles.heading}>Privacy</h1>
        {/* TODO(real data): launch date for this page. */}

        <div className={styles.body}>
          <h2>Your journal</h2>
          <p>
            Your journal is for you. Entries are stored encrypted, and only you can see them in the app.
            You can edit or delete an entry whenever you like. Deleting an entry removes it from Soulwe.
          </p>
          <p>
            When you save an entry, its text is sent to an AI provider, Anthropic or Google (Gemini),
            which writes a reflection. The reflection is saved with your entry, also encrypted.
          </p>
          {/* TODO(real data): provider retention and training use; do not publish until reviewed. */}

          <h2>Circles</h2>
          <p>
            Circles are anonymous. You get a different anonymous name in each circle, and other members see
            that name, not your real name or account.
          </p>
          <p>
            Everything you post in a circle can be seen by everyone who has joined it. Please share only
            what you are comfortable with others reading.
          </p>
          <p>
            You can delete your own messages. When you leave a circle you can choose to delete your
            messages there as well. If you don't, they stay in the circle under your anonymous name.
            Messages cannot be edited.
          </p>
          {/* TODO(real data): circle message retention, reporting and moderation. */}

          <h2>Where Soulwe runs</h2>
          <p>Soulwe is hosted on Vercel and Render.</p>
          {/* TODO(real data): data storage region. */}

          {/* TODO(real data): account data collected, hosting, and account deletion. */}

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
