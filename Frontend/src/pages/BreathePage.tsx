import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { NotebookPen } from 'lucide-react'
import { isApiError, type BreathingExercise, type BreathingSession } from '@/types'
import { api } from '@/lib/api'
import styles from './BreathePage.module.css'

interface Phase { label: string; duration: number }

interface BreathingConfig {
  id: string
  slug: string
  name: string
  description: string
  phases: Phase[]
}

const DESCRIPTIONS: Record<string, string> = {
  '478': 'Inhale 4s, hold 7s, exhale 8s. A longer exhale for a slower pace.',
  box: 'Inhale 4s, hold 4s, exhale 4s, hold 4s. A steady, even rhythm.',
}

// Catalog stores three durations; box adds a second hold.
function buildConfig(ex: BreathingExercise): BreathingConfig {
  const phases: Phase[] = [
    { label: 'Inhale', duration: ex.inhale_s },
    { label: 'Hold',   duration: ex.hold_s },
    { label: 'Exhale', duration: ex.exhale_s },
  ]
  if (ex.slug === 'box' && ex.hold_s) {
    phases.push({ label: 'Hold', duration: ex.hold_s })
  }
  return {
    id: ex.id,
    slug: ex.slug,
    name: ex.name,
    description: DESCRIPTIONS[ex.slug] ?? ex.description,
    phases,
  }
}

function fmtDuration(totalS: number) {
  const m = Math.floor(totalS / 60)
  const s = totalS % 60
  return m > 0 ? `${m}m ${s}s` : `${s}s`
}

function fmtClock(totalS: number) {
  const clamped = Math.max(0, totalS)
  const m = Math.floor(clamped / 60)
  const s = clamped % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(
    () => typeof window !== 'undefined' && !!window.matchMedia
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false,
  )
  useEffect(() => {
    if (!window.matchMedia) return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  return reduced
}

const LENGTHS = [1, 3, 5] as const

export default function BreathePage() {
  const [exercises, setExercises]       = useState<BreathingConfig[] | null>(null)
  const [exercisesError, setExercisesError] = useState<string | null>(null)
  const [reloadTick, setReloadTick]     = useState(0)
  const [selectedId, setSelectedId]     = useState<string | null>(null)
  const [running, setRunning]           = useState(false)
  const [paused, setPaused]             = useState(false)
  const [completed, setCompleted]       = useState(false)
  const [minutes, setMinutes]           = useState<number>(3)
  const [phaseIdx, setPhaseIdx]         = useState(0)
  const [secs, setSecs]                 = useState(0)
  const [sessionRemaining, setSessionRemaining] = useState(0)
  const [breathCount, setBreathCount]   = useState(0)
  const [saving, setSaving]             = useState(false)
  const [saveError, setSaveError]       = useState<string | null>(null)
  const [savedSummary, setSavedSummary] = useState<BreathingSession | null>(null)
  const timerRef       = useRef<ReturnType<typeof setInterval> | null>(null)
  const startedAtRef   = useRef(0)
  const recordingRef   = useRef(false)

  const reducedMotion = usePrefersReducedMotion()

  // Latest-value refs so the timer never reads stale data.
  const configRef = useRef<BreathingConfig | null>(null)
  configRef.current = exercises?.find(e => e.id === selectedId) ?? null

  // Mirror `secs`; deriving the phase in a setState updater would double-count breaths.
  const secsRef = useRef(0)
  // Session clock keeps the total budget across pause/resume.
  const remainingRef = useRef(0)
  // Tracks the phase that seeded the countdown so resume does not reseed.
  const seededRef = useRef('')

  const config = configRef.current
  const phase  = config ? config.phases[phaseIdx % config.phases.length] : null

  useEffect(() => {
    let cancelled = false
    setExercises(null)
    setExercisesError(null)
    api.breathe.exercises()
      .then(res => {
        if (cancelled) return
        const configs = res.exercises.map(buildConfig)
        setExercises(configs)
        if (configs.length > 0) {
          setSelectedId(prev => (prev && configs.some(c => c.id === prev) ? prev : configs[0].id))
        }
      })
      .catch(err => {
        if (cancelled) return
        setExercisesError(
          isApiError(err) ? err.message : 'We could not load breathing exercises right now. Please try again.',
        )
      })
    return () => { cancelled = true }
  }, [reloadTick])

  const start = () => {
    const cfg = configRef.current
    if (!cfg) return
    const first = cfg.phases[0]
    setPhaseIdx(0)
    seededRef.current = `${cfg.id}:0`
    secsRef.current = first.duration
    setSecs(first.duration)
    remainingRef.current = minutes * 60
    setSessionRemaining(minutes * 60)
    setBreathCount(0)
    setPaused(false)
    setCompleted(false)
    setSaveError(null)
    setSavedSummary(null)
    startedAtRef.current = Date.now()
    setRunning(true)
  }

  const pause = () => {
    if (!running || paused) return
    setPaused(true)
  }

  const resume = () => {
    if (!running || !paused) return
    setPaused(false)
  }

  const stop = () => {
    setRunning(false)
    setPaused(false)
    setCompleted(false)
    if (timerRef.current) clearInterval(timerRef.current)
    setPhaseIdx(0)
    seededRef.current = ''
    secsRef.current = 0
    setSecs(0)
    remainingRef.current = 0
    setSessionRemaining(0)
  }

  const switchTechnique = (id: string) => {
    if (id === selectedId) return
    stop()
    setSelectedId(id)
  }

  // Only explicit Stop records; zero-breath runs are skipped.
  const recordSession = async () => {
    const exerciseId = configRef.current?.id
    if (!exerciseId || breathCount < 1) return
    recordingRef.current = true
    setSaving(true)
    setSaveError(null)
    const durationS = Math.max(1, Math.round((Date.now() - startedAtRef.current) / 1000))
    try {
      const res = await api.breathe.sessions.record({
        exercise_id: exerciseId,
        breaths: breathCount,
        duration_s: durationS,
      })
      setSavedSummary(res.session)
    } catch (err) {
      setSaveError(
        isApiError(err) && err.status === 404
          ? 'That exercise is no longer available.'
          : isApiError(err)
            ? err.message
            : 'We could not save your session. Please try again.',
      )
    } finally {
      setSaving(false)
      recordingRef.current = false
    }
  }

  const handleStopClick = () => {
    if (recordingRef.current) return
    stop()
    void recordSession()
  }

  // Session and phase clocks pause/resume together.
  useEffect(() => {
    if (!running || paused || !configRef.current) return
    const phases = configRef.current.phases
    const cfgId = configRef.current.id
    const phaseDuration = phases[phaseIdx % phases.length].duration
    const token = `${cfgId}:${phaseIdx}`
    if (seededRef.current !== token) {
      seededRef.current = token
      secsRef.current = phaseDuration
      setSecs(phaseDuration)
    }

    timerRef.current = setInterval(() => {
      remainingRef.current -= 1
      setSessionRemaining(remainingRef.current)
      if (remainingRef.current <= 0) {
        if (timerRef.current) clearInterval(timerRef.current)
        seededRef.current = ''
        secsRef.current = 0
        setSecs(0)
        setPhaseIdx(0)
        setRunning(false)
        setPaused(false)
        setCompleted(true)
        return
      }

      const remaining = secsRef.current - 1
      if (remaining > 0) {
        secsRef.current = remaining
        setSecs(remaining)
        return
      }
      // One-shot effects live here, not inside a setState updater.
      if (timerRef.current) clearInterval(timerRef.current)
      const nextIdx   = phaseIdx + 1
      const nextPhase = phases[nextIdx % phases.length]
      if (nextPhase.label === 'Inhale') setBreathCount(c => c + 1)
      // Set here too so the number never flashes to zero.
      secsRef.current = nextPhase.duration
      setSecs(nextPhase.duration)
      setPhaseIdx(nextIdx)
    }, 1000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [running, paused, phaseIdx, selectedId])

  useEffect(() => () => {
    if (timerRef.current) clearInterval(timerRef.current)
  }, [])

  const phases = config?.phases ?? []
  const prevLabel = phases.length
    ? phases[(phaseIdx - 1 + phases.length) % phases.length].label
    : ''
  let discScale = 1
  if (running && phase) {
    if (phase.label === 'Inhale') discScale = 1.25
    else if (phase.label === 'Exhale') discScale = 0.82
    else if (phase.label === 'Hold') discScale = prevLabel === 'Exhale' ? 0.82 : 1.25
  }
  if (reducedMotion) discScale = 1

  const totalSessionSeconds = minutes * 60
  const elapsedSessionSeconds = Math.max(0, totalSessionSeconds - sessionRemaining)
  const sessionProgress = totalSessionSeconds > 0
    ? Math.min(100, (elapsedSessionSeconds / totalSessionSeconds) * 100)
    : 0

  return (
    <div className={styles.page}>
      <div className={styles.shell}>
        {exercises !== null && exercises.length > 0 && (
          <div
            className={styles.techniqueBar}
            role="radiogroup"
            aria-label="Breathing technique"
          >
            {exercises.map(t => (
              <label
                key={t.id}
                className={[
                  styles.techniqueTab,
                  selectedId === t.id ? styles.techniqueTabActive : '',
                ].join(' ')}
              >
                <input
                  type="radio"
                  name="breathe-technique"
                  value={t.id}
                  checked={selectedId === t.id}
                  onChange={() => switchTechnique(t.id)}
                  className="sr-only"
                />
                {t.name}
              </label>
            ))}
          </div>
        )}

        {!config && !exercisesError && exercises === null && (
          <p className={styles.statusNote} role="status">Loading breathing exercises…</p>
        )}

        {!config && exercisesError && (
          <div className={styles.errorNote} role="alert">
            <p>Couldn’t load breathing exercises. {exercisesError}</p>
            <button
              type="button"
              className={styles.retryBtn}
              onClick={() => setReloadTick(t => t + 1)}
            >
              Try again
            </button>
          </div>
        )}

        {!config && !exercisesError && exercises !== null && exercises.length === 0 && (
          <p className={styles.statusNote} role="status">
            No breathing exercises are available right now. Please check back soon.
          </p>
        )}

        {config && phase && (
          <>
            <div className={styles.hero}>
              <div className={styles.titleBlock}>
                <h1 className={styles.heading}>Breathe with me</h1>
                <p className={styles.sub}>
                  A moment of stillness. Even a few breaths change everything.
                </p>
              </div>

              <div className={styles.orbWrap}>
                <div
                  className={styles.orbPulse}
                  style={{
                    transform: `scale(${discScale})`,
                    transitionDuration: reducedMotion ? '0s' : running ? `${phase.duration}s` : '0.4s',
                  }}
                  aria-hidden="true"
                >
                  <div className={styles.orbGlow} />
                  <div className={styles.orbBody} />
                </div>
                <div className={styles.orbContent}>
                  {running ? (
                    <>
                      <p className={styles.phaseLabel} aria-live="polite" aria-atomic="true">
                        {phase.label}
                      </p>
                      <p className={styles.phaseCount}>{secs}</p>
                    </>
                  ) : (
                    <p className={styles.idlePrompt}>Ready to breathe</p>
                  )}
                </div>
              </div>

              <div className={styles.controls}>
                <div
                  className={styles.lengthGroup}
                  role="radiogroup"
                  aria-labelledby="breathe-length-label"
                >
                  <span id="breathe-length-label" className="sr-only">Session length</span>
                  {LENGTHS.map(m => (
                    <label
                      key={m}
                      className={[
                        styles.lengthOption,
                        minutes === m ? styles.lengthOptionActive : '',
                        running ? styles.lengthOptionDisabled : '',
                      ].join(' ')}
                    >
                      <input
                        type="radio"
                        name="breathe-length"
                        value={m}
                        checked={minutes === m}
                        onChange={() => setMinutes(m)}
                        disabled={running}
                        className="sr-only"
                      />
                      {m} min
                    </label>
                  ))}
                </div>

                <div className={styles.actionRow}>
                  {!running && (
                    <button
                      type="button"
                      className={styles.primaryBtn}
                      onClick={start}
                      disabled={saving}
                      aria-label="Start breathing exercise"
                    >
                      {saving ? 'Saving…' : 'Start'}
                    </button>
                  )}
                  {running && !paused && (
                    <button type="button" className={styles.primaryBtn} onClick={pause}>
                      Pause
                    </button>
                  )}
                  {running && paused && (
                    <button type="button" className={styles.primaryBtn} onClick={resume}>
                      Resume
                    </button>
                  )}
                  {running && (
                    <button
                      type="button"
                      className={styles.resetBtn}
                      onClick={handleStopClick}
                      disabled={saving}
                      aria-label="Reset breathing exercise"
                    >
                      Reset
                    </button>
                  )}
                </div>

                <div className={styles.footer}>
                  <p className={styles.safetyNote}>
                    Breathe gently. If you feel dizzy or uncomfortable, stop and breathe normally.
                  </p>
                  <Link className={styles.journalLink} to="/journal">
                    <NotebookPen size={15} aria-hidden="true" />
                    <span>Finished? Write down how you feel in your journal.</span>
                  </Link>
                </div>

                {running && (
                  <div className={styles.progressRow}>
                    <div
                      className={styles.progressTrack}
                      role="progressbar"
                      aria-label="Session progress"
                      aria-valuemin={0}
                      aria-valuemax={totalSessionSeconds}
                      aria-valuenow={Math.round(elapsedSessionSeconds)}
                      aria-valuetext={`${fmtClock(sessionRemaining)} left`}
                    >
                      <span
                        className={styles.progressFill}
                        style={{ width: `${sessionProgress}%` }}
                      />
                    </div>
                    <span className={styles.remainingText}>{fmtClock(sessionRemaining)} left</span>
                  </div>
                )}

                {saving && (
                  <div className={styles.savingLine} role="status">Saving your session…</div>
                )}
                {!saving && saveError && (
                  <div className={styles.saveError} role="alert">
                    We couldn’t save your session — it wasn’t recorded. {saveError}
                  </div>
                )}
                {!saving && !saveError && savedSummary && (
                  <div className={styles.saveLine} role="status">
                    <span aria-hidden="true">✓ </span>
                    Saved — {savedSummary.name ?? config.name} · {savedSummary.breaths} breath
                    {savedSummary.breaths !== 1 ? 's' : ''} · {fmtDuration(savedSummary.duration_s)}
                  </div>
                )}

                {completed && (
                  <div className={styles.completion} role="status">
                    <p>Well done. Take a moment.</p>
                    <Link to="/home">Check in with your mood</Link>
                  </div>
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
