// 'unauthenticated' is still fully usable: personal features run against an anonymous session this provider bootstraps, and registration returns no token.

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

  // Any 401 on an authenticated request (expired/invalid token) drops to unauthenticated.
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setUser(null)
      setStatus('unauthenticated')
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  // Signing out must not strand the anonymous session the personal features use.
  useEffect(() => {
    if (status !== 'unauthenticated') return
    void ensureAnonSession().catch(() => {
      // Best-effort: the real error surfaces at the personal request, and this is single-flight.
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