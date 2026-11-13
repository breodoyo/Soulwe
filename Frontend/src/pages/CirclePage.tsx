import {
  Fragment,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentType,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import {
  Feather, Briefcase, Home, HeartHandshake, Sprout, MessageCircle, Lock,
  Contrast, Church, BookOpen, Trash2, X,
} from 'lucide-react'
import { api } from '@/lib/api'
import { clearAnonToken, ensureAnonSession, getAccessToken } from '@/lib/api'
import { useAuth } from '@/auth/AuthContext'
import { isApiError, type Circle, type CircleMessage } from '@/types'
import styles from './CirclePage.module.css'

const MESSAGE_PAGE_SIZE = 25
const MAX_MESSAGE_LENGTH = 1000
const CONVERSATION_STARTERS = [
  'How are you arriving today?',
  'One small thing about my day',
]

function HalfSunIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M3 16.5h18" />
      <path d="M7.5 16.5a4.5 4.5 0 0 1 9 0" />
      <path d="M12 4.5v2.2" />
      <path d="M5.6 7.1 7.2 8.7" />
      <path d="m18.4 7.1-1.6 1.6" />
    </svg>
  )
}

const CIRCLE_ICONS: Record<string, ComponentType> = {
  grief: Feather,
  work: Briefcase,
  family: Home,
  relationships: HeartHandshake,
  growth: Sprout,
  trauma: Contrast,
  faith: Church,
  anxiety: HalfSunIcon,
}

function circleIcon(key: string | null) {
  return (key && CIRCLE_ICONS[key]) || MessageCircle
}

function errorMessage(err: unknown, fallback: string): string {
  return isApiError(err) ? err.message : fallback
}

function initialsOf(name: string): string {
  return name.split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase()
}

function formatMessageTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const minutes = Math.max(0, Math.round((Date.now() - date.getTime()) / 60000))
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return date.toLocaleTimeString('en-KE', { hour: '2-digit', minute: '2-digit' })
}

// Full time for the tooltip and screen readers; visible labels stay relative.
function formatMessageTimeFull(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('en-KE', { dateStyle: 'medium', timeStyle: 'short' })
}

function dayKey(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return date.toDateString()
}

function formatDayDivider(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const today = new Date()
  const yesterday = new Date()
  yesterday.setDate(today.getDate() - 1)
  if (date.toDateString() === today.toDateString()) return 'Today'
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday'
  const options: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }
  if (date.getFullYear() !== today.getFullYear()) options.year = 'numeric'
  return date.toLocaleDateString('en-KE', options)
}

interface MessageGroup {
  dateKey: string
  showDay: boolean
  messages: CircleMessage[]
}

function groupMessages(messages: CircleMessage[]): MessageGroup[] {
  const groups: MessageGroup[] = []
  for (const message of messages) {
    const key = dayKey(message.created_at)
    const previous = groups[groups.length - 1]
    const last = previous?.messages[previous.messages.length - 1]
    const sameDay = previous?.dateKey === key
    const sameAuthor = last?.author_name === message.author_name && last?.is_mine === message.is_mine
    if (previous && sameDay && sameAuthor) {
      previous.messages.push(message)
    } else {
      groups.push({
        dateKey: key,
        showDay: !previous || previous.dateKey !== key,
        messages: [message],
      })
    }
  }
  return groups
}

export default function CirclePage() {
  const { status: authStatus } = useAuth()
  const signedIn = authStatus === 'authenticated'

  const [sessionState, setSessionState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [sessionError, setSessionError] = useState<string | null>(null)
  const [sessionTick, setSessionTick] = useState(0)

  const [circles, setCircles] = useState<Circle[] | null>(null)
  const [circlesError, setCirclesError] = useState<string | null>(null)
  const [circlesTick, setCirclesTick] = useState(0)

  const [activeCircleId, setActiveCircleId] = useState<string | null>(null)
  const [detail, setDetail] = useState<Circle | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)

  const [messages, setMessages] = useState<CircleMessage[]>([])
  const [messagesLoading, setMessagesLoading] = useState(false)
  const [messagesError, setMessagesError] = useState<string | null>(null)
  const [nextCursor, setNextCursor] = useState<string | null>(null)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [olderError, setOlderError] = useState<string | null>(null)

  const [reply, setReply] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)

  const [joining, setJoining] = useState(false)
  const [leaving, setLeaving] = useState(false)
  const [leaveConfirm, setLeaveConfirm] = useState(false)
  const [removeMyMessages, setRemoveMyMessages] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [pageNotice, setPageNotice] = useState<string | null>(null)
  const [guidelinesOpen, setGuidelinesOpen] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<CircleMessage | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [messageMenuId, setMessageMenuId] = useState<string | null>(null)
  const [messageMenuPosition, setMessageMenuPosition] = useState<{ top: number; left: number } | null>(null)

  const bottomRef = useRef<HTMLDivElement>(null)
  const replyInputRef = useRef<HTMLTextAreaElement>(null)
  const messageMenuItemRef = useRef<HTMLButtonElement | null>(null)
  const scrollToBottomRef = useRef(true)
  const olderScrollRef = useRef<{ height: number; top: number } | null>(null)

  // Native <dialog> traps focus; restore the opener on close.
  const leaveDialogRef = useRef<HTMLDialogElement>(null)
  const leaveReturnFocusRef = useRef<HTMLElement | null>(null)
  const deleteDialogRef = useRef<HTMLDialogElement>(null)
  const deleteReturnFocusRef = useRef<HTMLElement | null>(null)
  const messageMenuTriggerRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    const dialog = leaveDialogRef.current
    if (!dialog) return
    if (leaveConfirm) {
      if (!dialog.open) dialog.showModal()
    } else if (dialog.open) {
      dialog.close()
      leaveReturnFocusRef.current?.focus()
      leaveReturnFocusRef.current = null
    }
  }, [leaveConfirm])

  useEffect(() => {
    const dialog = deleteDialogRef.current
    if (!dialog) return
    if (deleteTarget) {
      if (!dialog.open) dialog.showModal()
    } else if (dialog.open) {
      dialog.close()
      deleteReturnFocusRef.current?.focus()
      deleteReturnFocusRef.current = null
    }
  }, [deleteTarget])

  useEffect(() => {
    if (!messageMenuId) return
    const closeMenu = () => {
      setMessageMenuId(null)
      setMessageMenuPosition(null)
    }
    const handlePointerDown = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return
      if (event.target.closest('[data-message-menu]')) return
      closeMenu()
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      closeMenu()
      messageMenuTriggerRef.current?.focus()
    }
    const handleFocusIn = (event: FocusEvent) => {
      if (!(event.target instanceof Element)) return
      if (event.target === messageMenuTriggerRef.current) return
      const menu = document.getElementById(`message-menu-${messageMenuId}`)
      if (menu?.contains(event.target)) return
      closeMenu()
    }
    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    document.addEventListener('focusin', handleFocusIn)
    // A fixed menu would drift away from its trigger, so dismiss it on scroll.
    window.addEventListener('scroll', closeMenu, true)
    window.addEventListener('resize', closeMenu)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
      document.removeEventListener('focusin', handleFocusIn)
      window.removeEventListener('scroll', closeMenu, true)
      window.removeEventListener('resize', closeMenu)
    }
  }, [messageMenuId])

  useEffect(() => {
    if (!messageMenuId) return
    // Move focus to the action so keyboard users do not land on a hidden button.
    messageMenuItemRef.current?.focus({ preventScroll: true })
  }, [messageMenuId])

  useEffect(() => {
    if (!activeCircleId) return
    document.body.dataset.circleOpen = 'true'
    return () => {
      delete document.body.dataset.circleOpen
    }
  }, [activeCircleId])

  useEffect(() => {
    const input = replyInputRef.current
    if (!input) return
    input.style.height = 'auto'
    input.style.height = `${input.scrollHeight}px`
  }, [reply])

  const closeLeaveDialog = () => setLeaveConfirm(false)

  const closeDeleteDialog = () => {
    setDeleteTarget(null)
    setDeleteError(null)
  }

  const closeMessageMenu = (restoreFocus = false) => {
    setMessageMenuId(null)
    setMessageMenuPosition(null)
    if (restoreFocus) messageMenuTriggerRef.current?.focus()
  }

  const openMessageMenu = (event: ReactMouseEvent<HTMLButtonElement>, messageId: string) => {
    if (messageMenuId === messageId) {
      closeMessageMenu()
      return
    }
    const rect = event.currentTarget.getBoundingClientRect()
    const menuWidth = 168
    const menuHeight = 52
    const gap = 6
    const edge = 8
    const left = Math.max(edge, Math.min(rect.right - menuWidth, window.innerWidth - menuWidth - edge))
    const openUp = rect.bottom + gap + menuHeight > window.innerHeight - edge
    const top = openUp ? Math.max(edge, rect.top - menuHeight - gap) : rect.bottom + gap
    messageMenuTriggerRef.current = event.currentTarget
    setMessageMenuPosition({ top, left })
    setMessageMenuId(messageId)
  }

  const [sessionNotice, setSessionNotice] = useState<string | null>(null)

  // `err.credential` identifies which token api.ts already cleared.
  const handleSessionLost = (err: unknown, setError: (message: string) => void) => {
    if (isApiError(err) && err.credential === 'registered') {
      setError('Your session has ended. Sign in again to keep browsing circles.')
      return
    }
    // Also catches 'none', so a guest always recovers.
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
        // Only guests boot an anonymous session; minting one for a member would strand an identity.
        if (!getAccessToken()) {
          await ensureAnonSession()
        }
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
          errorMessage(err, "We could not start your session. Reload the page."),
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
          handleSessionLost(err, setCirclesError)
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
      const cursor = mode === 'older' ? messages[0]?.created_at : undefined
      const res = await api.circles.messages(circleId, {
        limit: MESSAGE_PAGE_SIZE,
        before: cursor,
      })
      // API pages arrive newest first; flip so newest lands at the bottom.
      const block = [...res.messages].reverse()
      if (mode === 'initial') {
        scrollToBottomRef.current = true
        setMessages(block)
      } else {
        const list = bottomRef.current?.parentElement
        if (list) olderScrollRef.current = { height: list.scrollHeight, top: list.scrollTop }
        scrollToBottomRef.current = false
        setMessages(prev => [...block, ...prev])
      }
      setNextCursor(res.next_cursor)
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        if (mode === 'initial') handleSessionLost(err, setMessagesError)
        else handleSessionLost(err, setOlderError)
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
    setRemoveMyMessages(false)
    setDeleteTarget(null)
    setDeleteError(null)
    setMessageMenuId(null)
    setMessageMenuPosition(null)
    setGuidelinesOpen(false)
    setPageNotice(null)
    try {
      const res = await api.circles.get(id)
      setDetail(res.circle)
      if (res.circle.is_member) {
        await loadMessages(id, 'initial')
      }
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost(err, setDetailError)
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
    setRemoveMyMessages(false)
    setDeleteTarget(null)
    setDeleteError(null)
    setMessageMenuId(null)
    setMessageMenuPosition(null)
    setGuidelinesOpen(false)
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
        handleSessionLost(err, setActionError)
        return
      }
      if (isApiError(err) && err.code === 'ALREADY_MEMBER') {
        // Already a member (e.g. a stale view) - treat as joined.
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
    const circleId = activeCircleId
    setLeaving(true)
    setActionError(null)
    try {
      await api.circles.leave(circleId, removeMyMessages)
      setDetail(prev => (prev ? { ...prev, is_member: false } : prev))
      setMessages([])
      setMessagesError(null)
      setNextCursor(null)
      setLeaveConfirm(false)
      setRemoveMyMessages(false)
      setMessageMenuId(null)
      setMessageMenuPosition(null)
      setPageNotice(
        removeMyMessages
          ? 'You left the circle and your messages here were deleted.'
          : 'You left the circle. You can rejoin anytime.',
      )
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost(err, setActionError)
        return
      }
      setActionError(errorMessage(err, 'We could not leave this circle right now. Please try again.'))
      setLeaveConfirm(false)
    } finally {
      setLeaving(false)
    }
  }

  const handleDeleteMessage = async () => {
    if (!deleteTarget || deleting || !activeCircleId) return
    const messageId = deleteTarget.id
    setDeleting(true)
    setDeleteError(null)
    setMessageMenuId(null)
    setMessageMenuPosition(null)
    try {
      await api.circles.deleteMessage(activeCircleId, messageId)
      setMessages(prev => prev.filter(m => m.id !== messageId))
      setDeleteTarget(null)
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost(err, setDeleteError)
        return
      }
      setDeleteError(
        isApiError(err) && err.status === 404
          ? 'This message could not be found. It may already be deleted.'
          : errorMessage(err, 'We could not delete your message right now. Please try again.'),
      )
    } finally {
      setDeleting(false)
    }
  }

  const handleSend = async () => {
    const text = reply.trim()
    if (!text || sending || !activeCircleId) return
    const circleId = activeCircleId
    setMessageMenuId(null)
    setMessageMenuPosition(null)
    setSending(true)
    setSendError(null)
    try {
      const res = await api.circles.sendMessage(circleId, text)
      scrollToBottomRef.current = true
      setMessages(prev => [...prev, res.message])
      setReply('')
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        handleSessionLost(err, setSendError)
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

  useLayoutEffect(() => {
    const previous = olderScrollRef.current
    if (!previous) return
    olderScrollRef.current = null
    const list = bottomRef.current?.parentElement
    if (list) list.scrollTop = list.scrollHeight - previous.height + previous.top
  }, [messages])

  useEffect(() => {
    if (!activeCircleId || messages.length === 0) return
    // Older pages must not yank the reader back to the newest message.
    if (!scrollToBottomRef.current) return
    scrollToBottomRef.current = false
    const list = bottomRef.current?.parentElement
    // Scroll the card viewport, not the page, past the header.
    if (list && list.scrollHeight > list.clientHeight + 1) {
      list.scrollTo({ top: list.scrollHeight, behavior: 'smooth' })
    } else {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [messages, activeCircleId])

  const messageGroups = groupMessages(messages)
  const memberLabel = detail
    ? detail.member_count === 0
      ? 'Be the first to join'
      : `${detail.member_count} ${detail.member_count === 1 ? 'member' : 'members'}`
    : ''

  if (sessionState === 'loading') {
    return (
      <div className={styles.page}>
        <p className={styles.status} role="status">
          {signedIn ? 'Opening the circles...' : 'Setting up your anonymous session...'}
        </p>
      </div>
    )
  }

  if (sessionState === 'error') {
    return (
      <div className={styles.page}>
        <div className={styles.heroWrap}>
          <img src="/circle.png.jpg" alt="Community circle" className={styles.heroImage} />
          <div className={styles.heroOverlay}>
            <div className={styles.heroContent}>
              <h1 className={styles.heroHeading}>Healing happens together.</h1>
              <p className={styles.heroSub}>Connect with people who understand, share experiences, and find strength in community.</p>
            </div>
          </div>
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
    <div className={activeCircleId ? `${styles.page} ${styles.pageThread}` : styles.page}>

      {!activeCircleId && (
        <>
          <div className={styles.heroWrap}>
            <img src="/circle.png.jpg" alt="Community circle" className={styles.heroImage} />
            <div className={styles.heroOverlay}>
              <div className={styles.heroContent}>
                <h1 className={styles.heroHeading}>Healing happens together.</h1>
                <p className={styles.heroSub}>Connect with people who understand, share experiences, and find strength in community.</p>
              </div>
            </div>
          </div>

          <div className={styles.layout}>
            <aside className={styles.sidebar}>
              <section className={styles.howItWorks} aria-labelledby="how-circles-work">
                <h2 id="how-circles-work" className={styles.howHeading}>How circles work</h2>
                <div className={styles.anonNotice} role="note">
                  <span className={styles.anonNoticeIcon} aria-hidden="true">
                    <Lock />
                  </span>
                  {signedIn ? (
                    <p>
                      You are posting as yourself. Guests in this circle stay anonymous, and
                      conversations stay within the circle.
                    </p>
                  ) : (
                    <p>All circles are anonymous. Your name is never shown. Conversations stay within the circle.</p>
                  )}
                </div>
              </section>
            </aside>

            <main className={styles.main}>
              {sessionNotice && <p className={styles.noticeInfo} role="status">{sessionNotice}</p>}

              <p className={styles.sectionLabel}>Open circles</p>

              {circles === null && (
                 <p className={styles.status} role="status">Loading circles...</p>
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
                <div className={styles.circleGrid}>
                  {circles.map(c => {
                    const Icon = circleIcon(c.icon)
                    return (
                      <button
                        key={c.id}
                        className={styles.circleCard}
                        onClick={() => openCircle(c.id)}
                        aria-label="Open"
                      >
                        <div className={styles.cardHeader}>
                          <span className={styles.circleIconTile} aria-hidden="true">
                            <Icon />
                          </span>
                          <div className={styles.circleInfo}>
                            <h2 className={styles.circleName}>{c.name}</h2>
                            {c.description && <p className={styles.circleDesc}>{c.description}</p>}
                          </div>
                        </div>
                        <div className={styles.cardFooter}>
                          <span className={styles.memberCount}>
                            {c.member_count === 0 ? "Be the first to join" : `${c.member_count} ${c.member_count === 1 ? "member" : "members"}`}
                          </span>
                          <span className={styles.cardAction}>Open</span>
                        </div>
                      </button>
                    )
                  })}
                </div>
              )}
            </main>
          </div>        </>
      )}

      {activeCircleId && (
        <div className={styles.thread}>
          <button type="button" className={styles.backBtn} onClick={closeCircle} aria-label="Back to circles">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
            Back to circles
          </button>

          <div className={styles.threadHeader}>
            <span className={`${styles.circleIconTile} ${styles.threadIconTile}`} aria-hidden="true">
              {(() => { const Icon = circleIcon(detail?.icon ?? null); return <Icon /> })()}
            </span>
            <div className={styles.threadHeaderMain}>
              <div className={styles.threadHeaderText}>
                <h1 className={styles.threadName}>{detail?.name ?? 'Circle'}</h1>
                {detail && (
                  <p className={styles.threadDesc}>
                    {detail.description ? `${detail.description} · ${memberLabel}` : memberLabel}
                  </p>
                )}
              </div>
              <div className={styles.headerActions}>
                <button
                  type="button"
                  className={styles.textAction}
                  onClick={() => setGuidelinesOpen(open => !open)}
                  aria-expanded={guidelinesOpen}
                  aria-controls="circle-guidelines"
                >
                  <BookOpen className={styles.textActionIcon} aria-hidden="true" />
                  Guidelines
                  <svg className={styles.textActionCaret} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <polyline points="6 9 12 15 18 9" />
                  </svg>
                </button>
                {!detailLoading && !detailError && detail && detail.is_member && (
                  <button
                    type="button"
                    className={styles.leaveAction}
                    onClick={e => {
                      // Capture the opener because focus may never move to the button.
                      leaveReturnFocusRef.current = e.currentTarget
                      setRemoveMyMessages(false)
                      setLeaveConfirm(true)
                    }}
                  >
                    Leave circle
                  </button>
                )}
              </div>
            </div>
          </div>

          {guidelinesOpen && (
            <section id="circle-guidelines" className={styles.guidelinesPanel} aria-labelledby="circle-guidelines-title">
              <div className={styles.guidelinesHeader}>
                <h2 id="circle-guidelines-title" className={styles.guidelinesHeading}>
                  Circle guidelines
                </h2>
                <button
                  type="button"
                  className={styles.guidelinesClose}
                  onClick={() => setGuidelinesOpen(false)}
                  aria-label="Hide guidelines"
                  title="Hide guidelines"
                >
                  <X aria-hidden="true" />
                </button>
              </div>
              {/* TODO(copy): placeholder wording, pending product review. Every line must
                  stay a true statement about how Soulwe works today — never promise
                  safety, confidentiality, or anonymity beyond what the code delivers. */}
              <ul className={styles.guidelineList}>
                <li>Everyone in this circle can see what you post.</li>
                <li>You appear under a name made just for this circle.</li>
                <li>Please don't share contact details.</li>
              </ul>
            </section>
          )}

          <div className={styles.threadMain}>

          {pageNotice && <p className={styles.noticeSuccess} role="status">{pageNotice}</p>}

          {detailLoading && <p className={styles.status} role="status">Loading circle...</p>}

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
                {joining ? 'Joining...' : 'Join this circle'}
              </button>
            </div>
          )}

          {!detailLoading && !detailError && detail && detail.is_member && (
            <section className={styles.conversationCard} aria-label="Conversation">
              <div className={styles.messageList} role="log" aria-label="Circle messages" aria-live="polite">
                {actionError && <p className={styles.errorText} role="alert">{actionError}</p>}
                {messagesLoading && <p className={styles.status} role="status">Loading messages...</p>}

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
                  <div className={styles.emptyState}>
                    <span className={styles.emptyIcon} aria-hidden="true">
                      <MessageCircle />
                    </span>
                    <h2 className={styles.emptyTitle}>A quiet space, for now.</h2>
                    <p className={styles.emptyText}>
                      No messages yet. Share what feels comfortable, when you're ready.
                    </p>
                  </div>
                )}

                {!messagesLoading && !messagesError && messages.length > 0 && (
                  <>
                    {nextCursor && (
                      <button
                        className={styles.loadOlderBtn}
                        onClick={() => loadMessages(activeCircleId, 'older')}
                        disabled={loadingOlder}
                      >
                        {loadingOlder ? 'Loading...' : 'Load older messages'}
                      </button>
                    )}
                    {olderError && <p className={styles.errorText} role="alert">{olderError}</p>}
                    {messageGroups.map(group => (
                      <Fragment key={group.messages[0].id}>
                        {group.showDay && (
                          <div className={styles.dayDivider} role="separator">
                            {formatDayDivider(group.messages[0].created_at)}
                          </div>
                        )}
                        <div className={styles.msgGroup}>
                          {group.messages.map((m, index) => {
                            const first = index === 0
                            const last = index === group.messages.length - 1
                            const hasReactions = Object.keys(m.reaction_counts).length > 0
                            return (
                              <div
                                key={m.id}
                                className={m.is_mine ? `${styles.msg} ${styles.msgMine}` : styles.msg}
                              >
                                {!m.is_mine && (
                                  first ? (
                                    <div className={styles.msgAvatar} aria-hidden="true">
                                      {initialsOf(m.author_name)}
                                    </div>
                                  ) : (
                                    <div className={styles.msgAvatarSpacer} aria-hidden="true" />
                                  )
                                )}
                                <div className={styles.msgBody}>
                                  <div className={styles.msgBubble}>
                                    {first && !m.is_mine && (
                                      <p className={styles.msgName}>{m.author_name}</p>
                                    )}
                                    <p className={styles.msgText}>{m.content}</p>
                                    {(last || hasReactions) && (
                                      <div className={styles.msgFooter}>
                                        {last && (
                                          <span className={styles.msgFooterLeft}>
                                            <span
                                              className={styles.msgTime}
                                              title={formatMessageTimeFull(m.created_at)}
                                            >
                                              <span className="sr-only">{formatMessageTimeFull(m.created_at)}</span>
                                              <span aria-hidden="true">{formatMessageTime(m.created_at)}</span>
                                            </span>
                                          </span>
                                        )}
                                        {hasReactions && (
                                          <div className={styles.msgReacts} aria-label="Reactions">
                                            {Object.entries(m.reaction_counts).map(([emoji, count]) => (
                                              <span key={emoji} className={styles.reactChip} aria-hidden="true">
                                                {emoji} {count}
                                              </span>
                                            ))}
                                          </div>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                  {m.is_mine && (
                                    <div className={styles.msgActions} data-message-menu>
                                      <button
                                        type="button"
                                        className={styles.msgMenuBtn}
                                        aria-label="Message actions"
                                        aria-haspopup="menu"
                                        aria-expanded={messageMenuId === m.id}
                                        aria-controls={messageMenuId === m.id ? `message-menu-${m.id}` : undefined}
                                        onClick={event => openMessageMenu(event, m.id)}
                                      >
                                        <span aria-hidden="true">⋯</span>
                                      </button>
                                      {messageMenuId === m.id && messageMenuPosition && (
                                        <div
                                          id={`message-menu-${m.id}`}
                                          className={styles.msgMenu}
                                          style={{ top: messageMenuPosition.top, left: messageMenuPosition.left }}
                                          role="menu"
                                          aria-label="Message actions"
                                        >
                                          <button
                                            ref={messageMenuItemRef}
                                            type="button"
                                            role="menuitem"
                                            className={styles.msgMenuItem}
                                            onClick={() => {
                                              deleteReturnFocusRef.current = messageMenuTriggerRef.current
                                              closeMessageMenu()
                                              setDeleteError(null)
                                              setDeleteTarget(m)
                                            }}
                                          >
                                            <Trash2 aria-hidden="true" />
                                            Delete
                                          </button>
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              </div>
                            )
                          })}
                        </div>
                      </Fragment>
                    ))}
                  </>
                )}
                <div ref={bottomRef} />
              </div>

              {!messagesLoading && !messagesError && messages.length === 0 && reply.length === 0 && (
                <div className={styles.starters} aria-label="Conversation starters">
                  {CONVERSATION_STARTERS.map(starter => (
                    <button
                      key={starter}
                      type="button"
                      className={styles.starterBtn}
                      onClick={() => {
                        setReply(starter)
                        replyInputRef.current?.focus()
                      }}
                    >
                      {starter}
                    </button>
                  ))}
                </div>
              )}

              {sendError && (
                <p id="circle-reply-error" className={styles.composerError} role="alert">
                  {sendError}
                </p>
              )}

              <div className={styles.replyBox}>
                <label className="sr-only" htmlFor="circle-reply">
                  {signedIn ? 'Type your message' : 'Type your anonymous message'}
                </label>
                <span id="circle-reply-hint" className="sr-only">
                  Press Enter to send. Press Shift and Enter for a new line.
                </span>
                <textarea
                  id="circle-reply"
                  ref={replyInputRef}
                  className={styles.replyInput}
                  value={reply}
                  onChange={e => {
                    setReply(e.target.value)
                    if (sendError) setSendError(null)
                  }}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                      e.preventDefault()
                      void handleSend()
                    }
                  }}
                  placeholder={signedIn ? 'Share with the circle...' : 'Share anonymously...'}
                  aria-describedby={sendError ? 'circle-reply-hint circle-reply-error' : 'circle-reply-hint'}
                  aria-invalid={sendError ? true : undefined}
                  aria-keyshortcuts="Enter"
                  enterKeyHint="send"
                  maxLength={MAX_MESSAGE_LENGTH}
                  rows={1}
                />
                <button
                  type="button"
                  className={styles.sendBtn}
                  onClick={handleSend}
                  disabled={sending || !reply.trim()}
                  aria-disabled={sending || !reply.trim()}
                  aria-label="Send message"
                  aria-busy={sending}
                  title="Send message"
                >
                  {sending
                    ? <span className={styles.sendSpinner} aria-hidden="true" />
                    : (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" aria-hidden="true">
                        <line x1="22" y1="2" x2="11" y2="13" />
                        <polygon points="22 2 15 22 11 13 2 9 22 2" />
                      </svg>
                    )}
                </button>
              </div>
              {sending && (
                <span className="sr-only" role="status">Sending your message.</span>
              )}
            </section>
          )}

          <p className={styles.urgentNote}>
            In immediate danger? Please contact your local emergency services or
            someone you trust.
          </p>
          </div>

          <dialog
            className={styles.leaveDialog}
            aria-labelledby="leave-dialog-title"
            ref={leaveDialogRef}
            onCancel={e => {
              e.preventDefault()
              closeLeaveDialog()
            }}
          >
            <h2 id="leave-dialog-title" className={styles.leaveDialogTitle}>
              Leave this circle?
            </h2>
            <p className={styles.leaveDialogText}>You can rejoin later.</p>
            <label className={styles.leaveDialogCheck}>
              <input
                type="checkbox"
                checked={removeMyMessages}
                onChange={e => setRemoveMyMessages(e.target.checked)}
                disabled={leaving}
              />
              <span>Also delete my messages in this circle</span>
            </label>
            <div className={styles.leaveDialogActions}>
              <button className={styles.leaveDialogCancel} onClick={closeLeaveDialog}>
                Cancel
              </button>
              <button
                className={styles.dangerBtn}
                onClick={handleLeave}
                disabled={leaving}
              >
                {leaving ? 'Leaving...' : 'Leave'}
              </button>
            </div>
          </dialog>

          <dialog
            className={styles.leaveDialog}
            aria-labelledby="delete-dialog-title"
            ref={deleteDialogRef}
            onCancel={e => {
              e.preventDefault()
              closeDeleteDialog()
            }}
          >
            <h2 id="delete-dialog-title" className={styles.leaveDialogTitle}>
              Delete message
            </h2>
            <p className={styles.leaveDialogText}>
              Delete this message? This can't be undone.
            </p>
            {deleteError && <p className={styles.errorText} role="alert">{deleteError}</p>}
            <div className={styles.leaveDialogActions}>
              <button className={styles.leaveDialogCancel} onClick={closeDeleteDialog}>
                Cancel
              </button>
              <button
                className={styles.dangerBtn}
                onClick={handleDeleteMessage}
                disabled={deleting}
              >
                {deleting ? 'Deleting...' : 'Delete'}
              </button>
            </div>
          </dialog>
        </div>
      )}
    </div>
  )
}















