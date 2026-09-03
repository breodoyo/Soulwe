import { Routes, Route } from 'react-router-dom'
import AppShell from '@/components/layout/AppShell'
import LandingPage from '@/pages/LandingPage'
import HomePage from '@/pages/HomePage'
import JournalPage from '@/pages/JournalPage'
import CirclePage from '@/pages/CirclePage'
import TherapistPage from '@/pages/TherapistPage'
import BreathePage from '@/pages/BreathePage'

export default function App() {
  return (
    <Routes>

      <Route path="/" element={<LandingPage />} />

      <Route element={<AppShell />}>
        <Route path="/home"      element={<HomePage />} />
        <Route path="/journal"   element={<JournalPage />} />
        <Route path="/circle"    element={<CirclePage />} />
        <Route path="/therapist" element={<TherapistPage />} />
        <Route path="/breathe"   element={<BreathePage />} />
      </Route>

    </Routes>
  )
}