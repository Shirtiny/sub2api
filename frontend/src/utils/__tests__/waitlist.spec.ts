import { describe, expect, it } from 'vitest'
import { normalizeWaitlistEmail } from '../waitlist'

describe('waiting-list mailbox syntax', () => {
  it.each(['person@example.com', '  Person+launch@EXAMPLE.COM \n', 'a.b@sub.example.co.uk'])('normalizes %s', email => {
    expect(normalizeWaitlistEmail(email)).toBe(email.trim().toLowerCase())
  })
  it.each(['', '   ', 'a', 'a@', '@example.com', 'a@localhost', 'a@@example.com', 'a b@example.com',
    'Name <a@example.com>', '.a@example.com', 'a.@example.com', 'a..b@example.com', 'a@-example.com',
    'a@example-.com', 'a@example..com', 'a@example.com.', 'a@127.0.0.1', 'a@example.c\nom', 'a@exa_mple.com',
    'a@例子.cn', `${'a'.repeat(65)}@example.com`, `a@${'x'.repeat(64)}.com`, `a@${'a.'.repeat(126)}com`
  ])('rejects %s', email => { expect(normalizeWaitlistEmail(email)).toBeNull() })
})
