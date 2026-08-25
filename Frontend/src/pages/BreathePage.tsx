import { useState, useEffect, useRef } from 'react'
import styles from './BreathePage.module.css'

type Technique = '478' | 'box'

interface Phase { label: string; duration: number }

const techniques: Record<Technique, { label: string; description: string; phases: Phase[] }> = {
  '478': {
    label: '4-7-8 breathing',
    description: 'Inhale 4s · Hold 7s · Exhale 8s — calms anxiety quickly',
    phases: [
      { label: 'Inhale',  duration: 4 },
      { label: 'Hold',    duration: 7 },
      { label: 'Exhale',  duration: 8 },
    ],
  },
  'box': {
    label: 'Box breathing',
    description: 'Inhale 4s · Hold 4s · Exhale 4s · Hold 4s — resets stress',
    phases: [
      { label: 'Inhale',  duration: 4 },
      { label: 'Hold',    duration: 4 },
      { label: 'Exhale',  duration: 4 },
      { label: 'Hold',    duration: 4 },
    ],
  },
}

export default function BreathePage() {
  const [technique, setTechnique] = useState<Technique>('478')
  const [running, setRunning] = useState(false)
  const [phaseIdx, setPhaseIdx] = useState(0)
  const [secs, setSecs] = useState(0)
  const [breathCount, setBreathCount] = useState(0)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const tech = techniques[technique]
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
    }
  }

  const stop = () => {
    setRunning(false)
    if (timerRef.current) clearInterval(timerRef.current)
    setPhaseIdx(0)
    setSecs(0)
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
          const nextIdx = phaseIdx + 1
          const nextPhase = techniques[technique].phases[nextIdx % techniques[technique].phases.length]
          if (nextPhase.label === 'Inhale') setBreathCount(c => c + 1)
          setPhaseIdx(nextIdx)
          return nextPhase.duration
        }
        return prev - 1
      })
    }, 1000)
    return () => { if (timerRef.current) clearInterval(timerRef.current) }
  }, [running, phaseIdx, technique])

  // Scale: inhale = big, exhale = normal, hold = stays
  const scale = running
    ? isInhale ? 1.45
    : isExhale ? 1
    : undefined   // hold — CSS keeps the current scale
    : 1

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Breathe with me</h1>
        <p className={styles.sub}>A moment of stillness. Even a few breaths change everything — <em>pumzika</em></p>
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
          <span className={styles.coreTech}>{technique === '478' ? '4·7·8' : 'Box'}</span>
        </div>
      </div>

      {/* Phase label + countdown */}
      <div className={styles.phaseDisplay} aria-live="polite" aria-atomic="true">
        <p className={styles.phaseLabel}>
          {running ? phase.label + '...' : breathCount > 0 ? 'Well done.' : 'Ready to begin'}
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
