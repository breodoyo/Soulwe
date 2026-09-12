import { useState, useEffect, useRef } from 'react'
import styles from './BreathePage.module.css'

type Technique = '478' | 'box'

interface Phase { label: string; duration: number; instruction: string }

const techniques: Record<Technique, {
  label: string
  description: string
  phases: Phase[]
}> = {
  '478': {
    label: '4-7-8 breathing',
    description: 'Inhale 4s · Hold 7s · Exhale 8s — calms anxiety quickly',
    phases: [
      { label: 'Inhale',  duration: 4, instruction: 'Breathe in slowly through your nose. Let your belly rise first, then your chest.' },
      { label: 'Hold',    duration: 7, instruction: 'Hold gently. Stay still. You are safe.' },
      { label: 'Exhale',  duration: 8, instruction: 'Release slowly through your mouth. Let everything go with the breath.' },
    ],
  },
  'box': {
    label: 'Box breathing',
    description: 'Inhale 4s · Hold 4s · Exhale 4s · Hold 4s — resets stress',
    phases: [
      { label: 'Inhale',  duration: 4, instruction: 'Breathe in through your nose. Slow and steady.' },
      { label: 'Hold',    duration: 4, instruction: 'Hold. Feel the stillness. You are grounded.' },
      { label: 'Exhale',  duration: 4, instruction: 'Breathe out through your mouth. Release the tension.' },
      { label: 'Hold',    duration: 4, instruction: 'Rest here. Empty and calm. You are okay.' },
    ],
  },
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
  const [technique, setTechnique]     = useState<Technique>('478')
  const [running, setRunning]         = useState(false)
  const [phaseIdx, setPhaseIdx]       = useState(0)
  const [secs, setSecs]               = useState(0)
  const [breathCount, setBreathCount] = useState(0)
  const [voiceOn, setVoiceOn]         = useState(true)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const tech  = techniques[technique]
  const phase = tech.phases[phaseIdx % tech.phases.length]

  const isInhale = phase.label === 'Inhale'
  const isExhale = phase.label === 'Exhale'

  const startStop = () => {
    if (running) {
      stop()
    } else {
      setPhaseIdx(0)
      setSecs(tech.phases[0].duration)
      setBreathCount(0)
      setRunning(true)
      if (voiceOn) speak(tech.phases[0].instruction)
    }
  }

  const stop = () => {
    setRunning(false)
    if (timerRef.current) clearInterval(timerRef.current)
    setPhaseIdx(0)
    setSecs(0)
    window.speechSynthesis?.cancel()
    if (voiceOn) speak('Well done. Take a moment to notice how you feel.')
  }

  const switchTechnique = (t: Technique) => {
    stop()
    setTechnique(t)
  }

  useEffect(() => {
    if (!running) return
    setSecs(phase.duration)
    timerRef.current = setInterval(() => {
      setSecs(prev => {
        if (prev <= 1) {
          clearInterval(timerRef.current!)
          const nextIdx   = phaseIdx + 1
          const nextPhase = techniques[technique].phases[nextIdx % techniques[technique].phases.length]
          if (nextPhase.label === 'Inhale') setBreathCount(c => c + 1)
          if (voiceOn) speak(nextPhase.instruction)
          setPhaseIdx(nextIdx)
          return nextPhase.duration
        }
        return prev - 1
      })
    }, 1000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [running, phaseIdx, technique])

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
            {technique === '478' ? '4·7·8' : 'Box'}
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
          onClick={startStop}
          aria-label={running ? 'Stop breathing exercise' : 'Start breathing exercise'}
        >
          {running ? 'Stop' : 'Start'}
        </button>
      </div>

      {/* Technique selector */}
      <div className={styles.techniqueList} role="list">
        {(Object.entries(techniques) as [Technique, typeof techniques[Technique]][]).map(([key, t]) => (
          <button
            key={key}
            className={[styles.techniqueItem, technique === key ? styles.techniqueItemActive : ''].join(' ')}
            onClick={() => switchTechnique(key)}
            role="listitem"
            aria-pressed={technique === key}
          >
            <div className={styles.techNum}>{key === '478' ? '1' : '2'}</div>
            <div className={styles.techInfo}>
              <p className={styles.techName}>{t.label}</p>
              <p className={styles.techDesc}>{t.description}</p>
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}