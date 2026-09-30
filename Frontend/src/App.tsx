import { Routes, Route, Navigate } from 'react-router-dom'
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

// Soulwe is publicly browsable. No route redirects a guest to /login: the
// landing page, the shell, and every page inside it open for anyone. Features
// that store the user's own data gate just that part of the page in place
// (see SignInPrompt), and therapist communication is the one action family
// that needs a registered account.
export default function App() {
  return (
    <Routes>

      {/* Public */}
      <Route path="/" element={<LandingPage />} />
      <Route path="/login"    element={<GuestOnly><LoginPage /></GuestOnly>} />
      <Route path="/register" element={<GuestOnly><RegisterPage /></GuestOnly>} />

      {/* App area (shared shell) — browsable by guests and anonymous users */}
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
