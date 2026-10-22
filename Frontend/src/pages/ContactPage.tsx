import { Link } from 'react-router-dom'
import styles from './LegalPages.module.css'

// Stub. No contact channel is invented; the real one is filled in before launch.
export default function ContactPage() {
  return (
    <div className={styles.page}>
      <div className={styles.inner}>
        <h1 className={styles.heading}>Contact</h1>

        <div className={styles.body}>
          <p>
            For questions about Soulwe, your data, or therapist listings, reach
            the team at:
          </p>
          <p className={styles.todo}>
            TODO(real data): contact email address, phone line, or contact form
            endpoint. Do not invent one.
          </p>
          <p>
            If you are in crisis, Soulwe is not an emergency service. See the
            crisis information at the bottom of every page.
          </p>
        </div>

        <Link className={styles.back} to="/">
          Back to the home page
        </Link>
      </div>
    </div>
  )
}
