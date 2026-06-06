interface APIErrorLike {
  code?: number | string
  message?: string
  reason?: string
  response?: {
    data?: {
      code?: number | string
      detail?: string
      message?: string
      reason?: string
    }
  }
}

type TranslateFn = (key: string) => string

function extractErrorMessage(error: unknown): string {
  const err = (error || {}) as APIErrorLike
  return err.response?.data?.detail || err.response?.data?.message || err.message || ''
}

function extractErrorCode(error: unknown): string {
  const err = (error || {}) as APIErrorLike
  const code = err.reason ?? err.response?.data?.reason ?? err.code ?? err.response?.data?.code
  return code != null ? String(code) : ''
}

export function buildAuthErrorMessage(
  error: unknown,
  options: {
    fallback: string
    t?: TranslateFn
    namespace?: string
  }
): string {
  const { fallback, t, namespace } = options
  if (t && namespace) {
    const code = extractErrorCode(error)
    if (code) {
      const key = `${namespace}.${code}`
      const translated = t(key)
      if (translated && translated !== key) {
        return translated
      }
    }
  }

  const message = extractErrorMessage(error)
  return message || fallback
}
