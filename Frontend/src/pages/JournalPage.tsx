import { useState, useRef } from 'react'
import styles from './JournalPage.module.css'

type MoodTag = 'Anxious' | 'Hopeful' | 'Overwhelmed' | 'Grateful' | 'Lonely' | 'Calm' | 'Proud' | 'Grieving'

const moodTags: MoodTag[] = ['Anxious', 'Hopeful', 'Overwhelmed', 'Grateful', 'Lonely', 'Calm', 'Proud', 'Grieving']

const prompts = [
  { label: 'My day',           placeholder: "What's on your mind today..." },
  { label: 'Gratitude',        placeholder: 'Something I am grateful for is...' },
  { label: 'A challenge',      placeholder: 'A challenge I am facing is... and I feel...' },
  { label: 'Let go',           placeholder: 'Something I want to let go of is...' },
  { label: 'Family & pressure',placeholder: 'What my family expects of me versus what I feel inside...' },
  { label: 'My strength',      placeholder: 'What makes me feel strong as an African is...' },
]

const pastEntries = [
  { day: '18', month: 'Aug', preview: 'Talked to mama today. She still doesn\'t understand why I see a therapist but she hugged me after...', tags: ['Hopeful', 'Family'] },
  { day: '16', month: 'Aug', preview: 'Work pressure has been crushing. I keep smiling at the office but inside I feel like I\'m drowning.', tags: ['Overwhelmed', 'Anxious'] },
  { day: '14', month: 'Aug', preview: 'I did the breathing exercise three times today. Something shifted. Feeling calmer.', tags: ['Calm', 'Proud'] },
]

export default function JournalPage() {
  const [activePrompt, setActivePrompt] = useState(0)
  const [content, setContent] = useState('')
  const [selectedTags, setSelectedTags] = useState<MoodTag[]>([])
  const [reflection, setReflection] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const wordCount = content.trim() === '' ? 0 : content.trim().split(/\s+/).length
  const today = new Date().toLocaleDateString('en-KE', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })

  const toggleTag = (tag: MoodTag) => {
    setSelectedTags(prev =>
      prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]
    )
  }

  const handleSave = async () => {
    if (!content.trim()) {
      textareaRef.current?.focus()
      return
    }
    setLoading(true)
    setReflection(null)

    try {
      const res = await fetch('https://api.anthropic.com/v1/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: 'claude-sonnet-4-6',
          max_tokens: 1000,
          system: `You are a warm, culturally-aware mental health companion for Soulwe, an app for East Africans. You understand African family dynamics, the stigma around mental health, Ubuntu philosophy, and the balance between communal expectations and personal wellbeing. Respond to journal entries with deep empathy — acknowledge feelings first, offer one gentle insight rooted in African wisdom or Ubuntu values, and end with a simple affirming sentence. Keep response to 3–4 sentences. Never be clinical or preachy. Write warmly, like a trusted elder who also understands modern life. No markdown, no asterisks.`,
          messages: [{
            role: 'user',
            content: `Journal entry: "${content}"\nMood tags: ${selectedTags.join(', ') || 'not selected'}\nPlease reflect back with warmth and cultural awareness.`
          }]
        })
      })
      const data = await res.json()
      const text = data.content?.map((c: { text?: string }) => c.text || '').join('') || ''
      setReflection(text || 'I hear you. What you\'re feeling matters deeply.')
    } catch {
      setReflection('I hear you. What you\'re feeling matters deeply. You\'ve already done something brave by putting words to it.')
    }

    setLoading(false)
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Your journal</h1>
        <p className={styles.sub}>Private, encrypted, only yours — <em>ya siri</em></p>
      </div>

      {/* Prompt chips */}
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

      {/* Write area */}
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
          <span className={styles.wordCount}>{wordCount} {wordCount === 1 ? 'word' : 'words'}</span>
        </div>
      </div>

      {/* Mood tags */}
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

      {/* Save button */}
      <button
        className={[styles.saveBtn, loading ? styles.saveBtnLoading : ''].join(' ')}
        onClick={handleSave}
        disabled={loading}
        aria-label="Save journal entry and get AI reflection"
      >
        {loading
          ? <><span className={styles.spinner} aria-hidden="true" /> Reflecting...</>
          : 'Save & reflect with AI'
        }
      </button>

      {/* AI reflection */}
      {reflection && (
        <div className={styles.reflection} role="region" aria-label="AI companion reflection" aria-live="polite">
          <p className={styles.reflectionEyebrow}>Your Soulwe companion</p>
          <p className={styles.reflectionText}>{reflection}</p>
          <div className={styles.reflectionActions}>
            <button className={styles.reflectionBtn} onClick={() => window.dispatchEvent(new CustomEvent('soulwe:prompt', { detail: 'Give me a grounding exercise for what I wrote in my journal' }))}>
              Grounding exercise ↗
            </button>
            <button className={styles.reflectionBtn} onClick={() => window.dispatchEvent(new CustomEvent('soulwe:prompt', { detail: 'Suggest an African proverb that speaks to what I am feeling' }))}>
              African proverb ↗
            </button>
            <button className={styles.reflectionBtn} onClick={() => window.dispatchEvent(new CustomEvent('soulwe:prompt', { detail: 'Help me reframe this challenge using Ubuntu philosophy' }))}>
              Ubuntu reframe ↗
            </button>
          </div>
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
