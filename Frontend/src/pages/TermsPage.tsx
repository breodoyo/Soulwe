import { Link } from 'react-router-dom'
import styles from './LegalPages.module.css'

// Stub. No terms are asserted here until real ones are written.
export default function TermsPage() {
  return (
    <div className={styles.page}>
      <div className={styles.inner}>
        <h1 className={styles.heading}>Terms</h1>
        <p className={styles.meta}>Last updated: TODO(real data: launch date)</p>

        <div className={styles.body}>
          <p>
            Soulwe offers anonymous peer support circles, a private journal,
            mood check-ins, guided breathing, and a directory of therapists who
            set their own session prices.
          </p>
          <p className={styles.todo}>
            TODO(real data): write the real terms of service with a lawyer,
            covering accounts, anonymous sessions, circle rules, booking and
            payment for therapist sessions, and what happens to data when an
            account is deleted. Do not publish this page until that is done.
          </p>
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
