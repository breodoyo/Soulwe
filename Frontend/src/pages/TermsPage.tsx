import { Link } from 'react-router-dom'
import styles from './LegalPages.module.css'

// Stub: no terms are asserted until real ones are written.
export default function TermsPage() {
  return (
    <div className={styles.page}>
      <div className={styles.inner}>
        <h1 className={styles.heading}>Terms</h1>
        {/* TODO(real data): launch date for this page. */}

        <div className={styles.body}>
          <p>
            Soulwe offers anonymous peer support circles, a private journal,
            mood check-ins, guided breathing, and a directory of therapists who
            set their own session prices.
          </p>
          {/* TODO(real data): lawyer-reviewed terms for accounts, anonymous sessions, circles, bookings and deletion; do not publish yet. */}
          <p>
            Soulwe is not a substitute for psychiatric care or emergency help.
            If you are in crisis, see the crisis information at the bottom of
            every page.
          </p>
        </div>

        <Link className={styles.back} to="/">
          Back to the home page
        </Link>
      </div>
    </div>
  )
}
