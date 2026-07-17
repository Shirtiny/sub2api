import {
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  type OpenAIWSMode
} from '@/utils/openaiWsMode'

export const AETHER_WS_SCHEMA_VERSION = 1
export const AETHER_WS_CONTROL_PROTOCOL = 'route-v1'
export const AETHER_WS_EXTRA_KEY = 'aether_ws'
export const OPENAI_APIKEY_WS_MODE_KEY = 'openai_apikey_responses_websockets_v2_mode'
export const OPENAI_APIKEY_WS_ENABLED_KEY = 'openai_apikey_responses_websockets_v2_enabled'

export interface AetherWSAccountConfig extends Record<string, unknown> {
  schema_version: typeof AETHER_WS_SCHEMA_VERSION
  enabled: boolean
  required_control_protocol: typeof AETHER_WS_CONTROL_PROTOCOL
}

export interface AetherWSAccountState {
  configured: boolean
  valid: boolean
  enabled: boolean
  reason: 'not_configured' | 'invalid_config' | 'field_conflict' | 'enabled' | 'disabled'
}

const asRecord = (value: unknown): Record<string, unknown> | null => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

export const readAetherWSAccountState = (
  extra: Record<string, unknown> | null | undefined
): AetherWSAccountState => {
  const configured = extra != null && Object.prototype.hasOwnProperty.call(extra, AETHER_WS_EXTRA_KEY)
  const config = asRecord(extra?.[AETHER_WS_EXTRA_KEY])
  if (!configured) {
    return {
      configured: false,
      valid: false,
      enabled: false,
      reason: 'not_configured'
    }
  }
  const enabledValue = config?.enabled
  const valid =
    config?.schema_version === AETHER_WS_SCHEMA_VERSION &&
    (enabledValue === true || enabledValue === false) &&
    config?.required_control_protocol === AETHER_WS_CONTROL_PROTOCOL
  if (!valid) {
    return {
      configured: true,
      valid: false,
      enabled: false,
      reason: 'invalid_config'
    }
  }
  const expectedMode = aetherWSManagedMode(enabledValue)
  const modeMatches = extra?.[OPENAI_APIKEY_WS_MODE_KEY] === expectedMode
  const enabledMirror = extra?.[OPENAI_APIKEY_WS_ENABLED_KEY]
  const mirrorMatches = enabledMirror === undefined || enabledMirror === enabledValue
  if (!modeMatches || !mirrorMatches) {
    return {
      configured: true,
      valid: false,
      enabled: false,
      reason: 'field_conflict'
    }
  }
  return {
    configured: true,
    valid: true,
    enabled: enabledValue,
    reason: enabledValue ? 'enabled' : 'disabled'
  }
}

export const aetherWSManagedMode = (enabled: boolean): OpenAIWSMode =>
  enabled ? OPENAI_WS_MODE_PASSTHROUGH : OPENAI_WS_MODE_OFF

export const applyAetherWSAccountConfig = (
  extra: Record<string, unknown> | null | undefined,
  enabled: boolean
): Record<string, unknown> => {
  const currentConfig = asRecord(extra?.[AETHER_WS_EXTRA_KEY]) ?? {}

  return {
    ...(extra ?? {}),
    [OPENAI_APIKEY_WS_MODE_KEY]: aetherWSManagedMode(enabled),
    [OPENAI_APIKEY_WS_ENABLED_KEY]: enabled,
    [AETHER_WS_EXTRA_KEY]: {
      ...currentConfig,
      schema_version: AETHER_WS_SCHEMA_VERSION,
      enabled,
      required_control_protocol: AETHER_WS_CONTROL_PROTOCOL
    } satisfies AetherWSAccountConfig
  }
}
