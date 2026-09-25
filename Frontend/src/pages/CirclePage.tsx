import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { clearAnonToken, getAnonToken, setAnonToken } from '@/lib/api'
import { isApiError, type Circle, type CircleMessage } from '@/types'
import styles from './CirclePage.module.css'

const MESSAGE_PAGE_SIZE = 25
const MAX_MESSAGE_LENGTH = 1000

const ANON_COLORS = ['#0F766E', '#D4780A', '#16A34A', '#44403C', '#78716C']

// The anonymous session is set up lazily when the circle experience first
// opens: reuse a stored token (validating it), otherwise create one. Only the
// anonymous token is ever persisted — never the registered JWT for these
// endpoints, and never anything else about the identity.
async function ensureAnonSession(): Promise<void> {
  const existing = getAnonToken()
  if (existing) {
    try {
      await api.anon.me()
      return
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        clearAnonToken()
      } else {
        throw err
      }
    }
  }
  const { anonymous_token } = await api.anon.create()
  setAnonToken(anonymous_token)
}

function errorMessage(err: unknown, fallback: string): string {
  return isApiError(err) ? err.message : fallback
}

function initialsOf(name: string): string {
  return name.split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase()
}

// Deterministic per-author colour so a message's bubble matches its avatar
// without ever exposing who the author is.
function avatarColor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  }
  return ANON_COLORS[hash % ANON_COLORS.length]
}

function formatMessageTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const minutes = Math.max(0, Math.round((Date.now() - date.getTime()) / 60000))
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return date.toLocaleDateString('en-KE', { day: 'numeric', month: 'short' })
}

export default function CirclePage() {
  // Anonymous session lifecycle
  const [sessionState, setSessionState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [sessionError, setSessionError] = useState<string | null>(null)
  const [sessionTick, setSessionTick] = useState(0)

  // Circle discovery
  const [circles, setCircles] = useState<Circle[] | null>(null)
  const [circlesError, setCirclesError] = useState<string | null>(null)
  const [circlesTick, setCirclesTick] = useState(0)

  // Circle detail / membership
  const [activeCircleId, setActiveCircleId] = useState<string | null>(null)
  const [detail, setDetail] = useState<Circle | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)

  // Message history (chronological: oldest → newest)
  const [messages, setMessages] = useState<CircleMessage[]>([])
  const [messagesLoading, setMessagesLoading] = useState(false)
  const [messagesError, setMessagesError] = useState<string | null>(null)
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [olderError, setOlderError] = useState<string | null>(null)

  // Sending
  const [reply, setReply] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)

  // Join / leave
  const [joining, setJoining] = useState(false)
  const [leaving, setLeaving] = useState(false)
  const [leaveConfirm, setLeaveConfirm] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [pageNotice, setPageNotice] = useState<string | null>(null)

  const bottomRef = useRef<HTMLDivElement>(null)

  const [sessionNotice, setSessionNotice] = useState<string | null>(null)

  // The anonymous token may have been invalidated by the server (e.g. rotated
  // by another device). Drop it, return to the circle list, and mint a fresh
  // session on the next pass.
  const handleSessionLost = () => {
    clearAnonToken()
    setSessionNotice('Your anonymous session was renewed. Please open the circle again.')
    setSessionTick(t => t + 1)
  }

  useEffect(() => {
    let cancelled = false
    setSessionState('loading')
    setSessionError(null)
    void (async () => {
      try {
        await ensureAnonSession()
        if (cancelled) return
        setSessionState('ready')
        setActiveCircleId(null)
        setDetail(null)
        setDetailError(null)
        setMessages([])
        setMessagesError(null)
        setNextCursor(null)
      } catch (err) {
        if (cancelled) return
        setSessionState('error')
        setSessionError(
          errorMessage(err, 'We could not set up your anonymous session right now. Please try again.'),
        )
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessionTick])

  useEffect(() => {
    if (sessionState !== 'ready') return
    let cancelled = false
    setCircles(null)
    setCirclesError(null)
    api.circles
      .list()
      .then(res => {
        if (cancelled) return
        setCircles(res.circles)
      })
      .catch(err => {
        if (cancelled) return
        if (isApiError(err) && err.status === 401) {
          handleSessionLost()
          return
        }
        setCirclesError(
          errorMessage(err, 'We could not load the circles right now. Please try again.'),
        )
      })
    return () => {
      cancelled = true
    }
  }, [sessionState, circlesTick])

  const loadMessages = async (circleId: string, mode: 'initial' | 'older') => {
    if (mode === 'initial') {
      setMessagesLoading(true)
      setMessagesError(null)
    } else {
      setLoadingOlder(true)
      setOlderError(null)
    }
    try {
      // `before` resumes from the oldest message currently loaded.
      const cursor = mode === 'older' ? messages[0]?.created_at : undefined
      const res = await api.circles.messages(circleId, {
        limit: MESSAGE_PAGE_SIZE,
        before: cursor,
      })
      // The API returns newest first; flip each page so the thread reads
      // oldest → newest, with the newest message at the bottom.
      const block = [...res.messages].reverse()
      if (mode === 'initial') {
        setMessages(block)
      } else {
        setMessages(prev => [...block, ...prev])
      }
      setNextCursor(res.next_cursor)
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost()
        return
      }
      if (mode === 'initial') {
        setMessagesError(
          errorMessage(err, 'We could not load the messages right now. Please try again.'),
        )
      } else {
        setOlderError(
          errorMessage(err, 'We could not load older messages right now. Please try again.'),
        )
      }
    } finally {
      if (mode === 'initial') {
        setMessagesLoading(false)
      } else {
        setLoadingOlder(false)
      }
    }
  }

  const openCircle = async (id: string) => {
    setActiveCircleId(id)
    setDetail(null)
    setDetailLoading(true)
    setDetailError(null)
    setMessages([])
    setMessagesError(null)
    setNextCursor(null)
    setOlderError(null)
    setActionError(null)
    setSendError(null)
    setLeaveConfirm(false)
    setPageNotice(null)
    try {
      const res = await api.circles.get(id)
      setDetail(res.circle)
      if (res.circle.is_member) {
        await loadMessages(id, 'initial')
      }
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost()
        return
      }
      setDetailError(
        errorMessage(err, 'We could not open this circle right now. Please try again.'),
      )
    } finally {
      setDetailLoading(false)
    }
  }

  const closeCircle = () => {
    setActiveCircleId(null)
    setDetail(null)
    setDetailError(null)
    setMessages([])
    setMessagesError(null)
    setNextCursor(null)
    setActionError(null)
    setSendError(null)
    setLeaveConfirm(false)
    setPageNotice(null)
  }

  const afterJoined = async (circleId: string) => {
    setDetail(prev => (prev ? { ...prev, is_member: true } : prev))
    setPageNotice('You joined the circle.')
    setActionError(null)
    await loadMessages(circleId, 'initial')
  }

  const handleJoin = async () => {
    if (joining || !activeCircleId) return
    const circleId = activeCircleId
    setJoining(true)
    setActionError(null)
    try {
      await api.circles.join(circleId)
      await afterJoined(circleId)
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost()
        return
      }
      if (isApiError(err) && err.code === 'ALREADY_MEMBER') {
        // Already a member (e.g. a stale view) — treat as joined.
        await afterJoined(circleId)
      } else {
        setActionError(
          errorMessage(err, 'We could not join this circle right now. Please try again.'),
        )
      }
    } finally {
      setJoining(false)
    }
  }

  const handleLeave = async () => {
    if (leaving || !activeCircleId) return
    if (!leaveConfirm) {
      setLeaveConfirm(true)
      return
    }
    const circleId = activeCircleId
    setLeaving(true)
    setActionError(null)
    try {
      await api.circles.leave(circleId)
      setDetail(prev => (prev ? { ...prev, is_member: false } : prev))
      setMessages([])
      setMessagesError(null)
      setNextCursor(null)
      setLeaveConfirm(false)
      setPageNotice('You left the circle. You can rejoin anytime.')
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost()
        return
      }
      setActionError(errorMessage(err, 'We could not leave this circle right now. Please try again.'))
      setLeaveConfirm(false)
    } finally {
      setLeaving(false)
    }
  }

  const handleSend = async () => {
    const text = reply.trim()
    if (!text || sending || !activeCircleId) return
    const circleId = activeCircleId
    setSending(true)
    setSendError(null)
    try {
      const res = await api.circles.sendMessage(circleId, text)
      setMessages(prev => [...prev, res.message])
      setReply('')
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost()
        return
      }
      setSendError(
        isApiError(err) && err.status === 429
          ? 'You are sending messages too quickly. Please wait a moment and try again.'
          : errorMessage(err, 'We could not send your message right now. Please try again.'),
      )
    } finally {
      setSending(false)
    }
  }

  useEffect(() => {
    if (activeCircleId && messages.length > 0) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, activeCircleId])

  if (sessionState === 'loading') {
    return (
      <div className={styles.page}>
        <p className={styles.status} role="status">Setting up your anonymous session…</p>
      </div>
    )
  }

  if (sessionState === 'error') {
    return (
      <div className={styles.page}>
        <div className={styles.header}>
          <h1 className={styles.heading}>Community circles</h1>
          <p className={styles.sub}>Peer support, African voices, safe space — <em>salama</em></p>
        </div>
        <div className={styles.errorNote} role="alert">
          <p>{sessionError}</p>
          <button className={styles.inlineBtn} onClick={() => setSessionTick(t => t + 1)}>
            Try again
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.page}>

      {/* Circle list */}
      {!activeCircleId && (
        <>
          <div className={styles.header}>
            <h1 className={styles.heading}>Community circles</h1>
            <p className={styles.sub}>Peer support, African voices, safe space — <em>salama</em></p>
          </div>

          {sessionNotice && <p className={styles.noticeInfo} role="status">{sessionNotice}</p>}

          <div className={styles.anonNotice} role="note">
            <span aria-hidden="true">🔒</span>
            <p>All circles are anonymous. Your name is never shown. Conversations stay within the circle.</p>
          </div>

          <p className={styles.sectionLabel}>Open circles</p>

          {circles === null && (
            <p className={styles.status} role="status">Loading circles…</p>
          )}

          {circles !== null && circlesError && (
            <div className={styles.errorNote} role="alert">
              <p>{circlesError}</p>
              <button className={styles.inlineBtn} onClick={() => setCirclesTick(t => t + 1)}>
                Try again
              </button>
            </div>
          )}

          {circles !== null && !circlesError && circles.length === 0 && (
            <p className={styles.emptyNote}>No circles are open right now. Please check back soon.</p>
          )}

          {circles !== null && !circlesError && circles.length > 0 && (
            <div className={styles.circleList}>
              {circles.map(c => (
                <button
                  key={c.id}
                  className={styles.circleCard}
                  onClick={() => openCircle(c.id)}
                  aria-label={`Open ${c.name}`}
                >
                  <span className={styles.circleIcon} aria-hidden="true">{c.icon ?? '💬'}</span>
                  <div className={styles.circleInfo}>
                    <h2 className={styles.circleName}>{c.name}</h2>
                    {c.description && <p className={styles.circleDesc}>{c.description}</p>}
                  </div>
                  <div className={styles.circleMeta}>
                    <span className={styles.memberCount}>
                      {c.member_count} {c.member_count === 1 ? 'member' : 'members'}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </>
      )}

      {/* Thread view */}
      {activeCircleId && (
        <div className={styles.thread}>
          <button className={styles.backBtn} onClick={closeCircle} aria-label="Back to circles">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
            Back to circles
          </button>

          <div className={styles.threadHeader}>
            <span className={styles.threadIcon} aria-hidden="true">{detail?.icon ?? '💬'}</span>
            <div>
              <h1 className={styles.threadName}>{detail?.name ?? 'Circle'}</h1>
              {detail?.description && <p className={styles.threadDesc}>{detail.description}</p>}
              {detail && (
                <p className={styles.threadMeta}>
                  {detail.member_count} {detail.member_count === 1 ? 'member' : 'members'}
                </p>
              )}
            </div>
          </div>

          {pageNotice && <p className={styles.noticeSuccess} role="status">{pageNotice}</p>}

          {detailLoading && <p className={styles.status} role="status">Loading circle…</p>}

          {!detailLoading && detailError && (
            <div className={styles.errorNote} role="alert">
              <p>{detailError}</p>
              <button className={styles.inlineBtn} onClick={() => openCircle(activeCircleId)}>
                Try again
              </button>
            </div>
          )}

          {!detailLoading && !detailError && detail && !detail.is_member && (
            <div className={styles.joinCard}>
              <p className={styles.joinText}>
                This circle is private to its members. Join to read the conversation and share with the group.
              </p>
              {actionError && <p className={styles.errorText} role="alert">{actionError}</p>}
              <button
                className={styles.joinBtn}
                onClick={handleJoin}
                disabled={joining}
              >
                {joining ? 'Joining…' : 'Join this circle'}
              </button>
            </div>
          )}

          {!detailLoading && !detailError && detail && detail.is_member && (
            <>
              <div className={styles.messageList} role="log" aria-label="Circle messages" aria-live="polite">
                {messagesLoading && <p className={styles.status} role="status">Loading messages…</p>}

                {!messagesLoading && messagesError && (
                  <div className={styles.errorNote} role="alert">
                    <p>{messagesError}</p>
                    <button
                      className={styles.inlineBtn}
                      onClick={() => loadMessages(activeCircleId, 'initial')}
                    >
                      Try again
                    </button>
                  </div>
                )}

                {!messagesLoading && !messagesError && messages.length === 0 && (
                  <p className={styles.emptyNote}>
                    No messages yet. Be the first to share something with this circle.
                  </p>
                )}

                {!messagesLoading && !messagesError && messages.length > 0 && (
                  <>
                    {nextCursor && (
                      <button
                        className={styles.loadOlderBtn}
                        onClick={() => loadMessages(activeCircleId, 'older')}
                        disabled={loadingOlder}
                      >
                        {loadingOlder ? 'Loading…' : 'Load older messages'}
                      </button>
                    )}
                    {olderError && <p className={styles.errorText} role="alert">{olderError}</p>}
                    {messages.map(m => (
                      <div key={m.id} className={styles.msg}>
                        <div
                          className={styles.msgAvatar}
                          style={{ background: avatarColor(m.anon_name) }}
                          aria-hidden="true"
                        >
                          {initialsOf(m.anon_name)}
                        </div>
                        <div className={styles.msgBubble}>
                          <p className={styles.msgName}>{m.anon_name}</p>
                          <p className={styles.msgText}>{m.content}</p>
                          <div className={styles.msgFooter}>
                            <span className={styles.msgTime}>{formatMessageTime(m.created_at)}</span>
                            {Object.keys(m.reaction_counts).length > 0 && (
                              <div className={styles.msgReacts} aria-label="Reactions">
                                {Object.entries(m.reaction_counts).map(([emoji, count]) => (
                                  <span key={emoji} className={styles.reactChip} aria-hidden="true">
                                    {emoji} {count}
                                  </span>
                                ))}
                              </div>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                  </>
                )}
                <div ref={bottomRef} />
              </div>

              {sendError && <p className={styles.errorText} role="alert">{sendError}</p>}
              {actionError && <p className={styles.errorText} role="alert">{actionError}</p>}

              <div className={styles.leaveRow}>
                {leaveConfirm ? (
                  <>
                    <span className={styles.leavePrompt}>Leave this circle?</span>
                    <button
                      className={styles.dangerBtn}
                      onClick={handleLeave}
                      disabled={leaving}
                    >
                      {leaving ? 'Leaving…' : 'Confirm leave'}
                    </button>
                    <button
                      className={styles.inlineBtn}
                      onClick={() => setLeaveConfirm(false)}
                    >
                      Stay
                    </button>
                  </>
                ) : (
                  <button className={styles.leaveBtn} onClick={handleLeave}>
                    Leave circle
                  </button>
                )}
              </div>

              <div className={styles.replyBox}>
                <label className="sr-only" htmlFor="circle-reply">Type your anonymous message</label>
                <textarea
                  id="circle-reply"
                  className={styles.replyInput}
                  value={reply}
                  onChange={e => setReply(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault()
                      void handleSend()
                    }
                  }}
                  placeholder="Share something anonymously..."
                  maxLength={MAX_MESSAGE_LENGTH}
                  rows={1}
                />
                <button
                  className={styles.sendBtn}
                  onClick={handleSend}
                  disabled={sending || !reply.trim()}
                  aria-label="Send message"
                >
                  {sending
                    ? <span className={styles.sendSpinner} aria-hidden="true" />
                    : (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                        <line x1="22" y1="2" x2="11" y2="13" />
                        <polygon points="22 2 15 22 11 13 2 9 22 2" />
                      </svg>
                    )}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}