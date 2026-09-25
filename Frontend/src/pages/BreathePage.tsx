import { useState, useEffect, useRef } from 'react'
import { isApiError, type BreathingExercise, type BreathingSession } from '@/types'
import { api } from '@/lib/api'
import styles from './BreathePage.module.css'

interface Phase { label: string; duration: number; instruction: string }

interface BreathingConfig {
  id: string
  slug: string
  name: string
  description: string
  phases: Phase[]
}

// Voice guidance scripts (frontend copy — the backend only exposes durations).
// Timings and display metadata come from the API; only slugs '478' and 'box'
// exist in the curated catalog, so these scripts keyed by slug cover all seeds.
interface ScriptSet { inhale: string; hold: string; exhale: string; rest?: string }

const SCRIPTS: Record<string, ScriptSet> = {
  '478': {
    inhale: 'Breathe in slowly through your nose. Let your belly rise first, then your chest.',
    hold: 'Hold gently. Stay still. You are safe.',
    exhale: 'Release slowly through your mouth. Let everything go with the breath.',
  },
  box: {
    inhale: 'Breathe in through your nose. Slow and steady.',
    hold: 'Hold. Feel the stillness. You are grounded.',
    exhale: 'Breathe out through your mouth. Release the tension.',
    rest: 'Rest here. Empty and calm. You are okay.',
  },
}

// The catalog stores three durations (inhale/hold/exhale); the curated 'box'
// exercise extends that pattern with a second hold, mirroring its seed data.
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

export default function BreathePage() {
  const [exercises, setExercises]       = useState<BreathingConfig[] | null>(null)
  const [exercisesError, setExercisesError] = useState<string | null>(null)
  const [reloadTick, setReloadTick]     = useState(0)
  const [selectedId, setSelectedId]     = useState<string | null>(null)
  const [running, setRunning]           = useState(false)
  const [phaseIdx, setPhaseIdx]         = useState(0)
  const [secs, setSecs]                 = useState(0)
  const [breathCount, setBreathCount]   = useState(0)
  const [voiceOn, setVoiceOn]           = useState(true)
  const [saving, setSaving]             = useState(false)
  const [saveError, setSaveError]       = useState<string | null>(null)
  const [savedSummary, setSavedSummary] = useState<BreathingSession | null>(null)
  const timerRef       = useRef<ReturnType<typeof setInterval> | null>(null)
  const startedAtRef   = useRef(0)
  const recordingRef   = useRef(false)

  // Latest-value refs so the long-lived timer effect never reads stale data
  // without having to list every stable value as a dependency.
  const configRef = useRef<BreathingConfig | null>(null)
  configRef.current = exercises?.find(e => e.id === selectedId) ?? null
  const voiceOnRef = useRef(voiceOn)
  voiceOnRef.current = voiceOn

  const config = configRef.current
  const phase  = config ? config.phases[phaseIdx % config.phases.length] : null

  const isInhale = phase?.label === 'Inhale'
  const isExhale = phase?.label === 'Exhale'

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
    if (!configRef.current) return
    setPhaseIdx(0)
    setSecs(configRef.current.phases[0].duration)
    setBreathCount(0)
    setSaveError(null)
    setSavedSummary(null)
    startedAtRef.current = Date.now()
    setRunning(true)
    if (voiceOnRef.current) speak(configRef.current.phases[0].instruction)
  }

  const stop = () => {
    setRunning(false)
    if (timerRef.current) clearInterval(timerRef.current)
    setPhaseIdx(0)
    setSecs(0)
    window.speechSynthesis?.cancel()
    if (voiceOnRef.current) speak('Well done. Take a moment to notice how you feel.')
  }

  const switchTechnique = (id: string) => {
    stop()
    setSelectedId(id)
  }

  // A session is only recorded on the user's explicit Stop — the backend's
  // POST /breathing/sessions records a completed session. Runs finished with
  // zero breaths (or abandoned by switching technique) are not recorded.
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

  useEffect(() => {
    if (!running || !configRef.current) return
    const phases = configRef.current.phases
    setSecs(phases[phaseIdx % phases.length].duration)
    timerRef.current = setInterval(() => {
      setSecs(prev => {
        if (prev <= 1) {
          if (timerRef.current) clearInterval(timerRef.current)
          const nextIdx   = phaseIdx + 1
          const nextPhase = phases[nextIdx % phases.length]
          if (nextPhase.label === 'Inhale') setBreathCount(c => c + 1)
          if (voiceOnRef.current) speak(nextPhase.instruction)
          setPhaseIdx(nextIdx)
          return nextPhase.duration
        }
        return prev - 1
      })
    }, 1000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [running, phaseIdx, selectedId])

  const scale = running
    ? isInhale ? 1.45
    : isExhale ? 1
    : undefined
    : 1

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Breathe with me</h1>
        <p className={styles.sub}>
          A moment of stillness. Even a few breaths change everything — <em>pumzika</em>
        </p>
      </div>

      {/* Voice toggle */}
      <div className={styles.voiceToggle}>
        <button
          className={[styles.voiceBtn, voiceOn ? styles.voiceBtnOn : ''].join(' ')}
          onClick={() => {
            setVoiceOn(v => !v)
            window.speechSynthesis?.cancel()
          }}
          aria-pressed={voiceOn}
          aria-label={voiceOn ? 'Turn voice guidance off' : 'Turn voice guidance on'}
        >
          {voiceOn ? '🔊 Voice on' : '🔇 Voice off'}
        </button>
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
          {/* Animated circle */}
          <div className={styles.circleWrap} aria-hidden="true">
            <div className={styles.ring1} />
            <div className={styles.ring2} />
            <div
              className={styles.core}
              style={{
                transform: scale !== undefined ? `scale(${scale})` : undefined,
                transitionDuration: running ? `${phase.duration}s` : '0.3s',
              }}
            >
              <span className={styles.coreTech}>
                {config.slug === '478' ? '4·7·8' : 'Box'}
              </span>
            </div>
          </div>

          {/* Phase label + countdown */}
          <div className={styles.phaseDisplay} aria-live="polite" aria-atomic="true">
            <p className={styles.phaseLabel}>
              {running
                ? phase.label + '...'
                : breathCount > 0
                  ? 'Well done.'
                  : 'Ready to begin'
              }
            </p>
            <p className={styles.phaseCount}>
              {running
                ? `${secs}s`
                : breathCount > 0
                  ? `${breathCount} breath${breathCount !== 1 ? 's' : ''} completed`
                  : 'Press start'
              }
            </p>
          </div>

          {/* Voice instruction text */}
          {running && (
            <div className={styles.instruction} aria-live="polite">
              <p className={styles.instructionText}>{phase.instruction}</p>
            </div>
          )}

          {/* Controls */}
          <div className={styles.controls}>
            <button
              className={[styles.startBtn, running ? styles.startBtnStop : ''].join(' ')}
              onClick={running ? handleStopClick : start}
              disabled={saving}
              aria-label={running ? 'Stop breathing exercise' : 'Start breathing exercise'}
            >
              {running ? 'Stop' : saving ? 'Saving…' : 'Start'}
            </button>
          </div>

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

      {/* Technique selector */}
      {exercises !== null && exercises.length > 0 && (
        <div className={styles.techniqueList} role="list">
          {exercises.map((t, i) => (
            <button
              key={t.id}
              className={[styles.techniqueItem, selectedId === t.id ? styles.techniqueItemActive : ''].join(' ')}
              onClick={() => switchTechnique(t.id)}
              role="listitem"
              aria-pressed={selectedId === t.id}
            >
              <div className={styles.techNum}>{i + 1}</div>
              <div className={styles.techInfo}>
                <p className={styles.techName}>{t.name}</p>
                <p className={styles.techDesc}>{t.description}</p>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}