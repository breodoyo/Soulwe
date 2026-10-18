import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const ANON_TOKEN_KEY = 'sw_anon_token'

async function loadApi() {
  const mod = await import('@/lib/api')
  return mod
}

describe('ensureAnonSession single-flight and recovery', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
    vi.restoreAllMocks()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  it('a test that two simultaneous bootstrap calls cause one network request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input)
      if (url.endsWith('/api/v1/auth/anonymous/me')) {
        return new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'auth required' } }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/auth/anonymous')) {
        return new Response(JSON.stringify({ anonymous_token: 'anon-token-1', anonymous_id: 'id-1' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } })
    })

    const { ensureAnonSession } = await loadApi()
    const p1 = ensureAnonSession()
    const p2 = ensureAnonSession()
    await Promise.all([p1, p2])

    // Count POSTs to /auth/anonymous
    const postCalls = fetchSpy.mock.calls.filter(([req]) => {
      const url = String(req)
      return url.endsWith('/api/v1/auth/anonymous') && (req as Request).method === 'POST'
    })
    expect(postCalls.length).toBe(1)
    expect(localStorage.getItem(ANON_TOKEN_KEY)).toBe('anon-token-1')
  })

  it('a repeated call with a valid token returns the same identity', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input)
      if (url.endsWith('/api/v1/auth/anonymous/me')) {
        return new Response(JSON.stringify({ anonymous_id: 'id-1' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/auth/anonymous')) {
        return new Response(JSON.stringify({ anonymous_token: 'fresh', anonymous_id: 'id-new' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200 })
    })

    const { ensureAnonSession, getAnonToken } = await loadApi()
    localStorage.setItem(ANON_TOKEN_KEY, 'existing-token')
    await ensureAnonSession()
    // Should have called me, not POSTed
    const postCalls = fetchSpy.mock.calls.filter(([req]) => {
      const url = String(req)
      return url.endsWith('/api/v1/auth/anonymous') && (req as Request).method === 'POST'
    })
    expect(postCalls.length).toBe(0)
    expect(getAnonToken()).toBe('existing-token')
  })

  it('rejected stored token is cleared and a fresh anonymous session is started once', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input)
      if (url.endsWith('/api/v1/auth/anonymous/me')) {
        return new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'auth required' } }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/auth/anonymous')) {
        return new Response(JSON.stringify({ anonymous_token: 'replacement', anonymous_id: 'id-new' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response('{}', { status: 200 })
    })

    const { ensureAnonSession } = await loadApi()
    localStorage.setItem(ANON_TOKEN_KEY, 'stale-token')
    await ensureAnonSession()
    // After 401 on me, it should have cleared and POSTed once
    const postCalls = fetchSpy.mock.calls.filter(([req]) => {
      const url = String(req)
      return url.endsWith('/api/v1/auth/anonymous') && (req as Request).method === 'POST'
    })
    expect(postCalls.length).toBe(1)
    expect(localStorage.getItem(ANON_TOKEN_KEY)).toBe('replacement')
    // Should not loop
    expect(postCalls.length).toBeLessThanOrEqual(1)
  })
})

