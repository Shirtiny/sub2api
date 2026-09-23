import apiClient from './client'

export async function joinWaitlist(email: string, turnstileToken?: string): Promise<void> {
  const { data } = await apiClient.post<{ accepted: boolean }>('/waitlist', {
    email, turnstile_token: turnstileToken || undefined
  }, { timeout: 60000 })
  if (data?.accepted !== true) throw new Error('Waitlist submission was not acknowledged')
}
