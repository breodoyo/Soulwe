import { useState, useRef } from 'react'
import styles from './JournalPage.module.css'

type MoodTag = 'Anxious' | 'Hopeful' | 'Overwhelmed' | 'Grateful' |
               'Lonely' | 'Calm' | 'Proud' | 'Grieving'

type JournalMode = 'journal' | 'prayer'

const moodTags: MoodTag[] = [
  'Anxious', 'Hopeful', 'Overwhelmed', 'Grateful',
  'Lonely', 'Calm', 'Proud', 'Grieving'
]

const prompts = [
  { label: 'My day',            placeholder: "What's on your mind today..."                          },
  { label: 'Gratitude',         placeholder: 'Something I am grateful for is...'                     },
  { label: 'A challenge',       placeholder: 'A challenge I am facing is... and I feel...'           },
  { label: 'Let go',            placeholder: 'Something I want to let go of is...'                   },
  { label: 'Family & pressure', placeholder: 'What my family expects of me versus what I feel...'    },
  { label: 'My strength',       placeholder: 'What makes me feel strong as an African is...'         },
]

const blessingVerses = [
  { text: '"The Lord bless you and keep you; the Lord make his face shine on you and be gracious to you."', ref: 'Numbers 6:24-25' },
  { text: '"Trust in the Lord with all your heart and lean not on your own understanding."',                 ref: 'Proverbs 3:5'    },
  { text: '"Be still, and know that I am God."',                                                             ref: 'Psalm 46:10'    },
  { text: '"Cast all your anxiety on him because he cares for you."',                                        ref: '1 Peter 5:7'    },
  { text: '"The Lord is close to the brokenhearted and saves those who are crushed in spirit."',             ref: 'Psalm 34:18'    },
  { text: '"For I know the plans I have for you, declares the Lord, plans to prosper you and not to harm you."', ref: 'Jeremiah 29:11' },
]

const pastEntries = [
  { day: '18', month: 'Aug', preview: 'Talked to mama today. She still doesn\'t understand why I see a therapist but she hugged me after...', tags: ['Hopeful', 'Family'], mode: 'journal' },
  { day: '16', month: 'Aug', preview: 'Lord, I am tired. I bring this job situation before you and ask for your peace...', tags: ['Calm'], mode: 'prayer' },
  { day: '14', month: 'Aug', preview: 'I did the breathing exercise three times today. Something shifted. Feeling calmer.', tags: ['Calm', 'Proud'], mode: 'journal' },
]

export default function JournalPage() {
  const [mode, setMode] = useState<JournalMode>('journal')

  // Journal mode state
  const [activePrompt, setActivePrompt] = useState(0)
  const [content, setContent] = useState('')
  const [selectedTags, setSelectedTags] = useState<MoodTag[]>([])

  // Prayer mode state
  const [gratitude, setGratitude] = useState('')
  const [petition, setPetition] = useState('')
  const [listening, setListening] = useState('')
  const [blessing] = useState(
    () => blessingVerses[Math.floor(Math.random() * blessingVerses.length)]
  )

  // Shared state
  const [reflection, setReflection] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const wordCount = content.trim() === '' ? 0 : content.trim().split(/\s+/).length
  const today = new Date().toLocaleDateString('en-KE', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
  })

  const switchMode = (m: JournalMode) => {
    setMode(m)
    setReflection(null)
  }

  const toggleTag = (tag: MoodTag) => {
    setSelectedTags(prev =>
      prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    )
  }

  const handleSave = async () => {
    const isPrayer = mode === 'prayer'
    const hasContent = isPrayer
      ? gratitude.trim() || petition.trim() || listening.trim()
      : content.trim()

    if (!hasContent) return
    setLoading(true)
    setReflection(null)

    const entryText = isPrayer
      ? `Gratitude: ${gratitude}\nPetition: ${petition}\nListening: ${listening}`
      : content

    const systemPrompt = isPrayer
      ? `You are a warm, faith-based spiritual companion for Soulwe, a mental health app for East Africans. The user has just written a prayer journal entry with three sections: gratitude, petition, and listening to God. Respond with deep warmth — affirm their faith, gently reflect one truth from what they wrote, and close with an encouraging word rooted in Scripture. Keep it to 3–4 sentences. Write like a trusted pastor or spiritual mentor, not a therapist. No markdown, no asterisks.`
      : `You are a warm, culturally-aware mental health companion for Soulwe, an app for East Africans. Respond to journal entries with deep empathy — acknowledge feelings first, offer one gentle insight rooted in African wisdom or Ubuntu values, and end with a simple affirming sentence. Keep response to 3–4 sentences. Never be clinical or preachy. No markdown, no asterisks.`

    try {
      const res = await fetch('https://api.anthropic.com/v1/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: 'claude-sonnet-4-6',
          max_tokens: 1000,
          system: systemPrompt,
          messages: [{
            role: 'user',
            content: `${isPrayer ? 'Prayer journal entry' : 'Journal entry'}: "${entryText}"`
          }]
        })
      })
      const data = await res.json()
      const text = data.content?.map((c: { text?: string }) => c.text || '').join('') || ''
      setReflection(text || 'Amen. Your heart has been heard.')
    } catch {
      setReflection(
        isPrayer
          ? 'Amen. Your words have been lifted up. Keep trusting — He hears every prayer.'
          : 'I hear you. What you\'re feeling matters deeply. You\'ve already done something brave by putting words to it.'
      )
    }

    setLoading(false)
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Your journal</h1>
        <p className={styles.sub}>Private, encrypted, only yours — <em>ya siri</em></p>
      </div>

      {/* Mode toggle */}
      <div className={styles.modeToggle} role="group" aria-label="Journal mode">
        <button
          className={[styles.modeBtn, mode === 'journal' ? styles.modeBtnActive : ''].join(' ')}
          onClick={() => switchMode('journal')}
        >
          ✍️ Journal
        </button>
        <button
          className={[styles.modeBtn, mode === 'prayer' ? styles.modeBtnActive : ''].join(' ')}
          onClick={() => switchMode('prayer')}
        >
          🙏 Prayer
        </button>
      </div>

      {/* ── JOURNAL MODE ── */}
      {mode === 'journal' && (
        <>
          <div className={styles.promptStrip} role="group" aria-label="Journal prompts">
            {prompts.map((p, i) => (
              <button
                key={p.label}
                className={[styles.promptChip, activePrompt === i ? styles.promptChipActive : ''].join(' ')}
                onClick={() => { setActivePrompt(i); textareaRef.current?.focus() }}
              >
                {p.label}
              </button>
            ))}
          </div>

          <div className={styles.writeBox}>
            <p className={styles.writeDate}>{today}</p>
            <textarea
              ref={textareaRef}
              className={styles.textarea}
              value={content}
              onChange={e => setContent(e.target.value)}
              placeholder={prompts[activePrompt].placeholder}
              aria-label="Journal entry"
              rows={6}
            />
            <div className={styles.writeFooter}>
              <span className={styles.wordCount}>
                {wordCount} {wordCount === 1 ? 'word' : 'words'}
              </span>
            </div>
          </div>

          <div>
            <p className={styles.sectionLabel}>How are you feeling?</p>
            <div className={styles.tagRow} role="group" aria-label="Mood tags">
              {moodTags.map(tag => (
                <button
                  key={tag}
                  className={[styles.tag, selectedTags.includes(tag) ? styles.tagActive : ''].join(' ')}
                  onClick={() => toggleTag(tag)}
                  aria-pressed={selectedTags.includes(tag)}
                >
                  {tag}
                </button>
              ))}
            </div>
          </div>
        </>
      )}

      {/* ── PRAYER MODE ── */}
      {mode === 'prayer' && (
        <div className={styles.prayerMode}>
          <p className={styles.prayerIntro}>
            "Do not be anxious about anything, but in every situation, by prayer and petition,
            with thanksgiving, present your requests to God." — Philippians 4:6
          </p>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="gratitude">
              🌿 Gratitude
              <span className={styles.prayerPrompt}>What am I thankful for today?</span>
            </label>
            <textarea
              id="gratitude"
              className={styles.prayerTextarea}
              value={gratitude}
              onChange={e => setGratitude(e.target.value)}
              placeholder="Lord, I am grateful for..."
              rows={3}
            />
          </div>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="petition">
              🕊️ Petition
              <span className={styles.prayerPrompt}>What am I bringing before God?</span>
            </label>
            <textarea
              id="petition"
              className={styles.prayerTextarea}
              value={petition}
              onChange={e => setPetition(e.target.value)}
              placeholder="Lord, I ask you for..."
              rows={3}
            />
          </div>

          <div className={styles.prayerSection}>
            <label className={styles.prayerLabel} htmlFor="listening">
              👂 Listening
              <span className={styles.prayerPrompt}>What do I feel He is saying to me?</span>
            </label>
            <textarea
              id="listening"
              className={styles.prayerTextarea}
              value={listening}
              onChange={e => setListening(e.target.value)}
              placeholder="I feel God is saying..."
              rows={3}
            />
          </div>

          {/* Closing blessing */}
          <div className={styles.blessingCard}>
            <p className={styles.blessingEyebrow}>A word for you today</p>
            <blockquote className={styles.blessingText}>{blessing.text}</blockquote>
            <p className={styles.blessingRef}>{blessing.ref}</p>
          </div>
        </div>
      )}

      {/* Save button — both modes */}
      <button
        className={[styles.saveBtn, loading ? styles.saveBtnLoading : ''].join(' ')}
        onClick={handleSave}
        disabled={loading}
        aria-label="Save and get AI reflection"
      >
        {loading
          ? <><span className={styles.spinner} aria-hidden="true" /> Reflecting...</>
          : mode === 'prayer' ? '🙏 Save prayer' : 'Save & reflect with AI'
        }
      </button>

      {/* AI reflection — both modes */}
      {reflection && (
        <div className={styles.reflection} role="region" aria-live="polite">
          <p className={styles.reflectionEyebrow}>
            {mode === 'prayer' ? 'A word from your companion' : 'Your Soulwe companion'}
          </p>
          <p className={styles.reflectionText}>{reflection}</p>
        </div>
      )}

      {/* Past entries */}
      <div>
        <p className={styles.sectionLabel}>Past entries</p>
        <div className={styles.entryList}>
          {pastEntries.map(entry => (
            <div key={entry.day} className={styles.entryItem}>
              <div className={styles.entryDate} aria-hidden="true">
                <span className={styles.entryDay}>{entry.day}</span>
                <span className={styles.entryMonth}>{entry.month}</span>
              </div>
              <div className={styles.entryBody}>
                <div className={styles.entryMeta}>
                  <span className={styles.entryModeTag}>
                    {entry.mode === 'prayer' ? '🙏 Prayer' : '✍️ Journal'}
                  </span>
                </div>
                <p className={styles.entryPreview}>{entry.preview}</p>
                <div className={styles.entryTags}>
                  {entry.tags.map(t => <span key={t} className={styles.entryTag}>{t}</span>)}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

    </div>
  )
}