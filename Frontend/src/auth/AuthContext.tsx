// Central authentication state for Soulwe.
//
// The provider owns the auth status machine:
//   loading        → restoring the session, nothing renders yet
//   authenticated  → a valid session exists (user is populated)
//   unauthenticated→ no account session. Soulwe is still fully usable: the
//                    personal features (journal, check-ins, breathing history)
//                    run against an anonymous session that this provider
//                    bootstraps, and pages that need an account (booking a
//                    therapist, the profile) ask for a sign-in in place rather
//                    than redirecting.
//
// Registration creates an account but returns no token (the backend contract),
// so it does not change the auth status. Logging in stores the token, and
// /auth/me + /users/me restore the session after a page refresh.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import type { User } from '@/types'
import { isApiError } from '@/types'
import { api, clearAuth, ensureAnonSession, getAccessToken, setUnauthorizedHandler } from '@/lib/api'

type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated'

interface AuthContextValue {
  status: AuthStatus
  user: User | null
  login: (email: string, password: string) => Promise<User>
  register: (email: string, password: string) => Promise<User>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading')
  const [user, setUser] = useState<User | null>(null)
  const restoredRef = useRef(false)

  const restoreSession = useCallback(async () => {
    if (!getAccessToken()) {
      setStatus('unauthenticated')
      return
    }

    try {
      // Validate the stored token, then hydrate the full profile.
      await api.auth.me()
      const { user } = await api.auth.profile()
      setUser(user)
      setStatus('authenticated')
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        clearAuth()
      }
      setStatus('unauthenticated')
    }
  }, [])

  useEffect(() => {
    if (restoredRef.current) return
    restoredRef.current = true
    void restoreSession()
  }, [restoreSession])

  // Any authenticated request that is later rejected with 401
  // (expired/invalid token) returns the app to the unauthenticated state.
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setUser(null)
      setStatus('unauthenticated')
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  // Signing out must not strand an anonymous session: the personal features
  // keep working exactly as they did before the user logged in, under the
  // anonymous session already in localStorage.
  useEffect(() => {
    if (status !== 'unauthenticated') return
    void ensureAnonSession().catch(() => {
      // Best-effort only. If the server is unreachable, the individual personal
      // request will surface the real error where it actually matters, and
      // ensureAnonSession is single-flight, so no storm of retries is spawned.
    })
  }, [status])

  const login = useCallback(async (email: string, password: string): Promise<User> => {
    const data = await api.auth.login(email, password)
    setUser(data.user)
    setStatus('authenticated')
    return data.user
  }, [])

  const register = useCallback(async (email: string, password: string): Promise<User> => {
    const data = await api.auth.register(email, password)
    return data.user
  }, [])

  const logout = useCallback(() => {
    clearAuth()
    setUser(null)
    setStatus('unauthenticated')
  }, [])

  const value = useMemo(
    () => ({ status, user, login, register, logout }),
    [status, user, login, register, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within an AuthProvider')
  return ctx
}