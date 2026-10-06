import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useEffect } from 'react'
import AppShell from '@/components/layout/AppShell'
import { GuestOnly } from '@/auth/RouteGuards'
import LandingPage from '@/pages/LandingPage'
import LoginPage from '@/pages/LoginPage'
import RegisterPage from '@/pages/RegisterPage'
import HomePage from '@/pages/HomePage'
import JournalPage from '@/pages/JournalPage'
import CirclePage from '@/pages/CirclePage'
import TherapistPage from '@/pages/TherapistPage'
import BreathePage from '@/pages/BreathePage'
import ProfilePage from '@/pages/ProfilePage'

// Set per route: an SPA never reloads, so a screen reader announcing a new page has nothing to read out otherwise.
const pageTitles: Record<string, string> = {
  '/':          'Soulwe — A home for your soul',
  '/login':     'Log in — Soulwe',
  '/register':  'Create an account — Soulwe',
  '/home':      'Home — Soulwe',
  '/journal':   'Journal — Soulwe',
  '/circle':    'Circle — Soulwe',
  '/therapist': 'Find a therapist — Soulwe',
  '/breathe':   'Breathe — Soulwe',
  '/profile':   'Profile — Soulwe',
}

export default function App() {
  const { pathname } = useLocation()

  useEffect(() => {
    document.title = pageTitles[pathname] ?? 'Soulwe'
  }, [pathname])

  return (
    <Routes>

      <Route path="/" element={<LandingPage />} />
      <Route path="/login"    element={<GuestOnly><LoginPage /></GuestOnly>} />
      <Route path="/register" element={<GuestOnly><RegisterPage /></GuestOnly>} />

      <Route element={<AppShell />}>
        <Route path="/home"      element={<HomePage />} />
        <Route path="/journal"   element={<JournalPage />} />
        <Route path="/circle"    element={<CirclePage />} />
        <Route path="/therapist" element={<TherapistPage />} />
        <Route path="/breathe"   element={<BreathePage />} />
        <Route path="/profile"   element={<ProfilePage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />

    </Routes>
  )
}
