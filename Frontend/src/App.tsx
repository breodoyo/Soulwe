import { Routes, Route, Navigate } from 'react-router-dom'
import AppShell from '@/components/layout/AppShell'
import { RequireAuth, GuestOnly } from '@/auth/RouteGuards'
import LandingPage from '@/pages/LandingPage'
import LoginPage from '@/pages/LoginPage'
import RegisterPage from '@/pages/RegisterPage'
import HomePage from '@/pages/HomePage'
import JournalPage from '@/pages/JournalPage'
import CirclePage from '@/pages/CirclePage'
import TherapistPage from '@/pages/TherapistPage'
import BreathePage from '@/pages/BreathePage'

export default function App() {
  return (
    <Routes>

      {/* Public */}
      <Route path="/" element={<LandingPage />} />
      <Route path="/login"    element={<GuestOnly><LoginPage /></GuestOnly>} />
      <Route path="/register" element={<GuestOnly><RegisterPage /></GuestOnly>} />

      {/* App area (shared shell) */}
      <Route element={<AppShell />}>
        {/* Registered-user features — require a valid session */}
        <Route path="/home"      element={<RequireAuth><HomePage /></RequireAuth>} />
        <Route path="/journal"   element={<RequireAuth><JournalPage /></RequireAuth>} />
        <Route path="/therapist" element={<RequireAuth><TherapistPage /></RequireAuth>} />
        <Route path="/breathe"   element={<RequireAuth><BreathePage /></RequireAuth>} />

        {/* Anonymous-only feature (circles). The anonymous-session flow is a
            later Phase 7 sub-phase; the page stays reachable for now. */}
        <Route path="/circle" element={<CirclePage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />

    </Routes>
  )
}