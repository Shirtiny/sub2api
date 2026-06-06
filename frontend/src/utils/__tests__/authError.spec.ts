import { describe, expect, it } from 'vitest'
import { buildAuthErrorMessage } from '@/utils/authError'

describe('buildAuthErrorMessage', () => {
  it('prefers response detail message when available', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            detail: 'detailed message',
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('detailed message')
  })

  it('falls back to response message when detail is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        response: {
          data: {
            message: 'plain message'
          }
        },
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('plain message')
  })

  it('falls back to error.message when response payload is unavailable', () => {
    const message = buildAuthErrorMessage(
      {
        message: 'error message'
      },
      { fallback: 'fallback' }
    )
    expect(message).toBe('error message')
  })

  it('prefers localized reason when translator and namespace are provided', () => {
    const t = (key: string) =>
      key === 'auth.errors.AFFILIATE_INVITE_LIMIT_REACHED'
        ? '此注册链接邀请人数已达到上限'
        : key

    const message = buildAuthErrorMessage(
      {
        reason: 'AFFILIATE_INVITE_LIMIT_REACHED',
        message: 'affiliate invite limit reached'
      },
      { fallback: 'fallback', t, namespace: 'auth.errors' }
    )

    expect(message).toBe('此注册链接邀请人数已达到上限')
  })

  it('uses fallback when no message can be extracted', () => {
    expect(buildAuthErrorMessage({}, { fallback: 'fallback' })).toBe('fallback')
  })
})
