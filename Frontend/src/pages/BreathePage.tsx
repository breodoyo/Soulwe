import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { Volume2, VolumeX } from 'lucide-react'
import { isApiError, type BreathingExercise, type BreathingSession } from '@/types'
import { api } from '@/lib/api'
import '@/styles/sunrise.css'
import styles from './BreathePage.module.css'

interface Phase { label: string; duration: number; instruction: string }

interface BreathingConfig {
  id: string
  slug: string
  name: string
  description: string
  phases: Phase[]
}

// Frontend-only voice copy; the backend exposes durations only. Each line is a
// complete sentence short enough to be spoken within the shortest phase, so the
// next phase's utterance never cuts it off mid-sentence.
interface ScriptSet { inhale: string; hold: string; exhale: string; rest?: string }

const SCRIPTS: Record<string, ScriptSet> = {
  '478': {
    inhale: 'Breathe in slowly through your nose.',
    hold: 'Hold gently. You are safe.',
    exhale: 'Breathe out slowly. Let everything go.',
  },
  box: {
    inhale: 'Breathe in slowly through your nose.',
    hold: 'Hold gently. Feel the stillness.',
    exhale: 'Breathe out slowly. Release the tension.',
    rest: 'Rest here. You are safe and calm.',
  },
}

// The catalog stores three durations; 'box' adds a second hold, mirroring its seed data.
function buildConfig(ex: BreathingExercise): BreathingConfig {
  const s = SCRIPTS[ex.slug] ?? SCRIPTS['478']
  const phases: Phase[] = [
    { label: 'Inhale', duration: ex.inhale_s, instruction: s.inhale },
    { label: 'Hold',   duration: ex.hold_s,   instruction: s.hold },
    { label: 'Exhale', duration: ex.exhale_s, instruction: s.exhale },
  ]
  if (ex.slug === 'box' && ex.hold_s) {
    phases.push({ label: 'Hold', duration: ex.hold_s, instruction: s.rest ?? s.hold })
  }
  return { id: ex.id, slug: ex.slug, name: ex.name, description: ex.description, phases }
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

// Speaks the instruction using the Web Speech API
function speak(text: string) {
  if (!window.speechSynthesis) return
  window.speechSynthesis.cancel() // stop any current speech
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.rate = 0.82      // slightly slower than normal — calm, unhurried
  utterance.pitch = 0.95     // slightly lower — warm, grounded
  utterance.volume = 1

  // Pick the best available voice — prefer a female English voice
  const voices = window.speechSynthesis.getVoices()
  const preferred = voices.find(v =>
    v.lang.startsWith('en') && v.name.toLowerCase().includes('female')
  ) || voices.find(v =>
    v.lang.startsWith('en')
  ) || voices[0]

  if (preferred) utterance.voice = preferred
  window.speechSynthesis.speak(utterance)
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
  const [runId, setRunId]               = useState(0)
  const [breathCount, setBreathCount]   = useState(0)
  const [voiceOn, setVoiceOn]           = useState(true)
  const [saving, setSaving]             = useState(false)
  const [saveError, setSaveError]       = useState<string | null>(null)
  const [savedSummary, setSavedSummary] = useState<BreathingSession | null>(null)
  const timerRef       = useRef<ReturnType<typeof setInterval> | null>(null)
  const startedAtRef   = useRef(0)
  const recordingRef   = useRef(false)

  const reducedMotion = usePrefersReducedMotion()

  // Latest-value refs so the long-lived timer effect never reads stale data.
  const configRef = useRef<BreathingConfig | null>(null)
  configRef.current = exercises?.find(e => e.id === selectedId) ?? null
  const voiceOnRef = useRef(voiceOn)
  voiceOnRef.current = voiceOn

  // What the timer reads, mirroring `secs`; deriving the phase from `prev` in a setState updater would count breaths and speech twice.
  const secsRef = useRef(0)
  // Session clock (total length in seconds) so pause/resume keeps the budget.
  const remainingRef = useRef(0)
  // Which phase seeded the current countdown; stops pause/resume restarting it.
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
    setRunId(r => r + 1)
    setRunning(true)
    if (voiceOnRef.current) speak(first.instruction)
  }

  const pause = () => {
    if (!running || paused) return
    setPaused(true)
    window.speechSynthesis?.cancel()
  }

  const resume = () => {
    if (!running || !paused) return
    setPaused(false)
    if (voiceOnRef.current && phase) speak(phase.instruction)
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
    window.speechSynthesis?.cancel()
    if (voiceOnRef.current) speak('Well done. Take a moment to notice how you feel.')
  }

  const switchTechnique = (id: string) => {
    if (id === selectedId) return
    stop()
    setSelectedId(id)
  }

  // Recorded only on an explicit Stop; runs with zero breaths are not, and never skipped for want of an account.
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

  // Timer: a session clock (total length) plus a per-phase clock. Pausing clears
  // the interval; resuming restarts it without reseeding the current phase.
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
        window.speechSynthesis?.cancel()
        if (voiceOnRef.current) speak('Well done. Take a moment to notice how you feel.')
        return
      }

      const remaining = secsRef.current - 1
      if (remaining > 0) {
        secsRef.current = remaining
        setSecs(remaining)
        return
      }
      // Everything that must happen exactly once lives here, not in a setState updater.
      if (timerRef.current) clearInterval(timerRef.current)
      const nextIdx   = phaseIdx + 1
      const nextPhase = phases[nextIdx % phases.length]
      if (nextPhase.label === 'Inhale') setBreathCount(c => c + 1)
      if (voiceOnRef.current) speak(nextPhase.instruction)
      // Reseeded by the effect, but set here too so the number never flashes to 0.
      secsRef.current = nextPhase.duration
      setSecs(nextPhase.duration)
      setPhaseIdx(nextIdx)
    }, 1000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [running, paused, phaseIdx, selectedId])

  // Leaving the page must stop timers and speech at once.
  useEffect(() => () => {
    if (timerRef.current) clearInterval(timerRef.current)
    window.speechSynthesis?.cancel()
  }, [])

  // Disc scales with the breath: small on exhale / hold-empty, larger on inhale / hold-full.
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

  const phaseText = running
    ? phase?.label ?? ''
    : breathCount > 0
      ? 'Well done.'
      : 'Ready to begin'

  return (
    <div className={[styles.page, 'sunrise'].join(' ')}>
      <div className={styles.container}>
        <div className={styles.layout}>
          <div className={styles.photoBand} aria-hidden="true" />

          <div className={styles.panel}>
            <div className={styles.header}>
              <h1 className={styles.heading}>Breathe with me</h1>
              <p className={styles.sub}>
                A moment of stillness. Even a few breaths change everything — <em>pumzika</em>
              </p>
            </div>

            {/* Load states */}
            {!config && !exercisesError && exercises === null && (
              <p className={styles.statusNote} role="status">Loading breathing exercises…</p>
            )}

            {!config && exercisesError && (
              <div className={styles.errorNote} role="alert">
                <p>Couldn’t load breathing exercises. {exercisesError}</p>
                <button
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
                {/* Orb: flat sun disc + one thin progress ring */}
                <div className={styles.orbWrap}>
                  <svg className={styles.ringSvg} viewBox="0 0 200 200" aria-hidden="true">
                    <circle className={styles.ringTrack} cx="100" cy="100" r="92" />
                    {running && !reducedMotion && (
                      <circle
                        key={`${selectedId}-${runId}-${phaseIdx}`}
                        className={styles.ringProgress}
                        cx="100"
                        cy="100"
                        r="92"
                        style={{
                          animationDuration: `${phase.duration}s`,
                          animationPlayState: paused ? 'paused' : 'running',
                        }}
                      />
                    )}
                  </svg>
                  <div
                    className={styles.disc}
                    style={{
                      transform: `scale(${discScale})`,
                      transitionDuration: running ? `${phase.duration}s` : '0.3s',
                    }}
                  >
                    <span className={styles.discTech}>
                      {config.slug === '478' ? '4·7·8' : 'Box'}
                    </span>
                  </div>
                </div>

                {/* Phase label + countdown (announced phase name only) */}
                {completed ? (
                  <div className={styles.completion}>
                    <p className={styles.completionTitle} role="status">
                      Well done. Take a moment.
                    </p>
                    <p className={styles.completionLinks}>
                      <Link to="/home">Check in with your mood</Link>
                      <span aria-hidden="true"> · </span>
                      <Link to="/journal">write in your journal</Link>
                    </p>
                  </div>
                ) : (
                  <div className={styles.phaseDisplay}>
                    <p className={styles.phaseLabel} aria-live="polite" aria-atomic="true">
                      {phaseText}
                    </p>
                    {running ? (
                      <p className={styles.phaseCount}>{secs}</p>
                    ) : (
                      <p className={styles.phaseCountSmall}>
                        {breathCount > 0
                          ? `${breathCount} breath${breathCount !== 1 ? 's' : ''} completed`
                          : 'Press start'
                        }
                      </p>
                    )}
                  </div>
                )}

                {/* Controls */}
                <div className={styles.controls}>
                  <button
                    className={[styles.voiceBtn, voiceOn ? styles.voiceBtnOn : ''].join(' ')}
                    onClick={() => {
                      setVoiceOn(v => !v)
                      window.speechSynthesis?.cancel()
                    }}
                    aria-pressed={voiceOn}
                    aria-label={voiceOn ? 'Turn voice guidance off' : 'Turn voice guidance on'}
                  >
                    {voiceOn
                      ? <><Volume2 aria-hidden="true" className={styles.voiceBtnIcon} /> Voice on</>
                      : <><VolumeX aria-hidden="true" className={styles.voiceBtnIcon} /> Voice off</>}
                  </button>

                  {!running && (
                    <button
                      className={styles.startBtn}
                      onClick={start}
                      disabled={saving}
                      aria-label="Start breathing exercise"
                    >
                      {saving ? 'Saving…' : 'Start'}
                    </button>
                  )}
                  {running && !paused && (
                    <button className={styles.pauseBtn} onClick={pause}>Pause</button>
                  )}
                  {running && paused && (
                    <button className={styles.startBtn} onClick={resume}>Resume</button>
                  )}
                  {running && (
                    <button
                      className={styles.stopBtn}
                      onClick={handleStopClick}
                      disabled={saving}
                      aria-label="Stop breathing exercise"
                    >
                      Stop
                    </button>
                  )}
                </div>

                {/* Length selector (radio semantics) */}
                <div className={styles.lengthRow}>
                  <span id="breathe-length-label" className={styles.lengthLabel}>Length</span>
                  <div
                    className={styles.lengthGroup}
                    role="radiogroup"
                    aria-labelledby="breathe-length-label"
                  >
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
                  {running && (
                    <span className={styles.remainingText}>{fmtClock(sessionRemaining)} left</span>
                  )}
                </div>

                {/* New copy: the one safety line */}
                <p className={styles.safety}>
                  If you feel dizzy or uncomfortable, stop and breathe normally.
                </p>

                {/* Save status — honest outcome of the backend call */}
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
              </>
            )}

            {/* Technique selector (radio semantics) */}
            {exercises !== null && exercises.length > 0 && (
              <div
                className={styles.techniqueList}
                role="radiogroup"
                aria-labelledby="breathe-techniques-label"
              >
                <span id="breathe-techniques-label" className="sr-only">Breathing techniques</span>
                {exercises.map((t, i) => (
                  <label
                    key={t.id}
                    className={[styles.techniqueItem, selectedId === t.id ? styles.techniqueItemActive : ''].join(' ')}
                  >
                    <input
                      type="radio"
                      name="breathe-technique"
                      value={t.id}
                      checked={selectedId === t.id}
                      onChange={() => switchTechnique(t.id)}
                      className="sr-only"
                    />
                    <span className={styles.techNum}>{i + 1}</span>
                    <span className={styles.techInfo}>
                      <span className={styles.techName}>{t.name}</span>
                      <span className={styles.techDesc}>{t.description}</span>
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
