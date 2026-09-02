import { useState, useRef, useEffect } from 'react'
import styles from './CirclePage.module.css'

interface Circle {
  id: string; slug: string; name: string
  description: string; icon: string; online: number
}

interface Message {
  id: string; anonName: string; color: string
  text: string; time: string; reactions: Record<string, number>
  mine?: boolean
}

const circles: Circle[] = [
  { id:'1', slug:'grief',         name:'Grief & loss circle',      description:'Navigating death and mourning in African families',             icon:'🕊️', online:8  },
  { id:'2', slug:'work',          name:'Work pressure circle',      description:'Burnout, hustle culture, and financial stress',                 icon:'💼', online:14 },
  { id:'3', slug:'family',        name:'Family expectations',       description:'When culture and mental health collide',                        icon:'🌿', online:6  },
  { id:'4', slug:'young',         name:'Young adults circle',       description:'Identity, relationships, and finding your path',                icon:'🌱', online:3  },
  { id:'5', slug:'trauma',        name:'Trauma & healing circle',   description:'A safe space for survivors of trauma, PTSD, and abuse',         icon:'💜', online:5  },
  { id:'6', slug:'relationships', name:'Relationships circle',      description:'Marriage, separation, infidelity, and loneliness in love',      icon:'💍', online:9  },
  { id:'7', slug:'faith',         name:'Faith & doubt circle',      description:'Spiritual struggles, unanswered prayers, and finding God',      icon:'🙏', online:4  },
  { id:'8', slug:'anxiety',       name:'Anxiety & depression',      description:'Panic, low mood, intrusive thoughts — you are not alone',       icon:'🧠', online:11 },
]

const seedMessages: Record<string, Message[]> = {
  grief: [
    { id:'1', anonName:'Anon Baobab',  color:'#C4714A', text:"Lost my father last month. The relatives keep saying 'be strong' but nobody asks if I'm okay. I'm exhausted from performing strength.", time:'2m ago',   reactions:{'💙':12,'🙏':4} },
    { id:'2', anonName:'Anon Willow',  color:'#4A6741', text:"I understand this so deeply. In our culture grief has a script — you have to mourn the right way. It's suffocating.",                  time:'5m ago',   reactions:{'💙':8}         },
    { id:'3', anonName:'Anon Savanna', color:'#5C3D2E', text:"What helped me was giving myself permission to grieve privately. You don't have to perform for anyone.",                               time:'8m ago',   reactions:{'💙':14,'✨':3}  },
  ],
  work: [
    { id:'1', anonName:'Anon Acacia',  color:'#C4714A', text:"Three jobs and still can't send money home. Feel like I'm failing everyone who believed in me.",                                        time:'1m ago',   reactions:{'🙏':22}         },
    { id:'2', anonName:'Anon River',   color:'#4A6741', text:"The pressure to be the 'family success story' is real. Therapy helped me separate my worth from my income.",                           time:'4m ago',   reactions:{'💙':17}         },
  ],
  family: [
    { id:'1', anonName:'Anon Sunrise', color:'#5C3D2E', text:"Mama says therapy is for people who hate their family. I'm doing it anyway but secretly. The guilt is heavy.",                         time:'just now', reactions:{'💙':9}          },
    { id:'2', anonName:'Anon Fern',    color:'#C4714A', text:"I told mine it was 'counselling for work stress.' They accepted that. Sometimes you translate mental health into language they can receive.", time:'3m ago', reactions:{'✨':31}       },
  ],
  young: [
    { id:'1', anonName:'Anon Ndovu',   color:'#4A6741', text:"Anyone else feel like you're between two worlds? Too westernised for home, too African for everywhere else?",                           time:'6m ago',   reactions:{'💙':19}         },
  ],
  trauma: [
    { id:'1', anonName:'Anon Cedar',   color:'#7C3D9E', text:"My PTSD makes me flinch at loud noises. My family thinks I'm being dramatic. It's been 2 years since the accident and they still don't understand.", time:'3m ago', reactions:{'💙':24,'🙏':8} },
    { id:'2', anonName:'Anon Flame',   color:'#C4714A', text:"You are not dramatic. What happened to you was real. The body keeps score long after the mind tries to move on. Keep going.", time:'5m ago', reactions:{'💙':31,'✨':12} },
    { id:'3', anonName:'Anon Stone',   color:'#4A6741', text:"EMDR therapy changed everything for me. If you can access it, please try. It took 6 months but I can finally sleep through the night.", time:'9m ago', reactions:{'🙏':18} },
  ],
  relationships: [
    { id:'1', anonName:'Anon Lotus',   color:'#C4714A', text:"My husband and I haven't spoken properly in 6 months. We just coexist. I don't know how to start the conversation without it becoming a fight.", time:'2m ago', reactions:{'💙':15,'🙏':6} },
    { id:'2', anonName:'Anon Pearl',   color:'#5C3D2E', text:"We went for couples counselling and it was the best decision we ever made. The therapist helped us hear each other for the first time.", time:'7m ago', reactions:{'✨':22} },
    { id:'3', anonName:'Anon Tide',    color:'#4A6741', text:"Separation is so lonely. Even when the marriage was painful, the silence of being alone is a different kind of pain.", time:'11m ago', reactions:{'💙':28,'🙏':14} },
  ],
  faith: [
    { id:'1', anonName:'Anon Ember',   color:'#C4714A', text:"I've been praying for 3 years about the same thing and nothing has changed. I'm starting to wonder if God is listening.", time:'4m ago', reactions:{'🙏':33,'💙':11} },
    { id:'2', anonName:'Anon Grace',   color:'#4A6741', text:"I went through the same season of silence. What helped me was shifting from asking God to fix things to asking Him to sit with me in it.", time:'6m ago', reactions:{'✨':41,'🙏':19} },
  ],
  anxiety: [
    { id:'1', anonName:'Anon Rain',    color:'#5C3D2E', text:"I had my first panic attack at work last week. I thought I was dying. My chest, my breathing — everything shut down. I'm scared it will happen again.", time:'1m ago', reactions:{'💙':21,'🙏':9} },
    { id:'2', anonName:'Anon Mist',    color:'#C4714A', text:"Panic attacks are terrifying but they cannot hurt you. The 4-7-8 breathing in this app helped me get through my last one. Try it next time you feel one coming.", time:'3m ago', reactions:{'💙':18,'✨':7} },
    { id:'3', anonName:'Anon Brook',   color:'#4A6741', text:"Depression makes every day feel like walking through mud. But I've learned to celebrate tiny wins — I got out of bed today. That is enough.", time:'8m ago', reactions:{'💙':44,'✨':22} },
  ],
}

const EMOJIS = ['💙', '🙏', '✨', '💪']
const ANON_COLORS = ['#C4714A', '#4A6741', '#5C3D2E', '#8C7B73']

export default function CirclePage() {
  const [activeCircle, setActiveCircle] = useState<Circle | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [reply, setReply] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  const openCircle = (circle: Circle) => {
    setActiveCircle(circle)
    setMessages(seedMessages[circle.slug] || [])
  }

  const closeCircle = () => {
    setActiveCircle(null)
    setMessages([])
  }

  const sendReply = () => {
    if (!reply.trim()) return
    const newMsg: Message = {
      id: Date.now().toString(),
      anonName: 'You (Anon)',
      color: ANON_COLORS[Math.floor(Math.random() * ANON_COLORS.length)],
      text: reply.trim(),
      time: 'just now',
      reactions: {},
      mine: true,
    }
    setMessages(prev => [...prev, newMsg])
    setReply('')
  }

  const react = (msgId: string, emoji: string) => {
    setMessages(prev => prev.map(m => {
      if (m.id !== msgId) return m
      const counts = { ...m.reactions }
      counts[emoji] = (counts[emoji] || 0) + 1
      return { ...m, reactions: counts }
    }))
  }

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  return (
    <div className={styles.page}>

      {/* Circle list */}
      {!activeCircle && (
        <>
          <div className={styles.header}>
            <h1 className={styles.heading}>Community circles</h1>
            <p className={styles.sub}>Peer support, African voices, safe space — <em>salama</em></p>
          </div>

          <div className={styles.anonNotice} role="note">
            <span aria-hidden="true">🔒</span>
            <p>All circles are anonymous. Your name is never shown. Conversations stay within the circle.</p>
          </div>

          <p className={styles.sectionLabel}>Active now</p>
          <div className={styles.circleList}>
            {circles.map(c => (
              <button
                key={c.id}
                className={styles.circleCard}
                onClick={() => openCircle(c)}
                aria-label={`Join ${c.name}`}
              >
                <span className={styles.circleIcon} aria-hidden="true">{c.icon}</span>
                <div className={styles.circleInfo}>
                  <h2 className={styles.circleName}>{c.name}</h2>
                  <p className={styles.circleDesc}>{c.description}</p>
                </div>
                <div className={styles.circleMeta}>
                  <span className={styles.onlineDot} aria-hidden="true" />
                  <span className={styles.onlineCount}>{c.online} online</span>
                </div>
              </button>
            ))}
          </div>
        </>
      )}

      {/* Thread view */}
      {activeCircle && (
        <div className={styles.thread}>
          <button className={styles.backBtn} onClick={closeCircle} aria-label="Back to circles">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <polyline points="15 18 9 12 15 6"/>
            </svg>
            Back to circles
          </button>

          <div className={styles.threadHeader}>
            <span aria-hidden="true">{activeCircle.icon}</span>
            <div>
              <h1 className={styles.threadName}>{activeCircle.name}</h1>
              <p className={styles.threadDesc}>{activeCircle.description}</p>
            </div>
          </div>

          <div className={styles.messageList} role="log" aria-label="Circle messages" aria-live="polite">
            {messages.map(msg => (
              <div key={msg.id} className={[styles.msg, msg.mine ? styles.msgMine : ''].join(' ')}>
                <div
                  className={styles.msgAvatar}
                  style={{ background: msg.color }}
                  aria-hidden="true"
                >
                  {msg.anonName.split(' ').map(w => w[0]).join('').slice(0, 2)}
                </div>
                <div className={styles.msgBubble}>
                  <p className={styles.msgName}>{msg.anonName}</p>
                  <p className={styles.msgText}>{msg.text}</p>
                  <div className={styles.msgFooter}>
                    <span className={styles.msgTime}>{msg.time}</span>
                    <div className={styles.msgReacts}>
                      {EMOJIS.map(e => (
                        <button
                          key={e}
                          className={styles.reactBtn}
                          onClick={() => react(msg.id, e)}
                          aria-label={`React with ${e}`}
                        >
                          {e} {msg.reactions[e] ? <span>{msg.reactions[e]}</span> : null}
                        </button>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>

          <div className={styles.replyBox}>
            <textarea
              className={styles.replyInput}
              value={reply}
              onChange={e => setReply(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  sendReply()
                }
              }}
              placeholder="Share something anonymously..."
              aria-label="Type your anonymous message"
              rows={1}
            />
            <button className={styles.sendBtn} onClick={sendReply} aria-label="Send message">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                <line x1="22" y1="2" x2="11" y2="13"/>
                <polygon points="22 2 15 22 11 13 2 9 22 2"/>
              </svg>
            </button>
          </div>
        </div>
      )}
    </div>
  )
}