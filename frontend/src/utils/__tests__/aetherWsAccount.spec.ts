import { describe, expect, it } from 'vitest'
import {
  AETHER_WS_CONTROL_PROTOCOL,
  AETHER_WS_SCHEMA_VERSION,
  applyAetherWSAccountConfig,
  readAetherWSAccountState
} from '@/utils/aetherWsAccount'

describe('aetherWsAccount utils', () => {
  it('atomically enables passthrough and preserves unknown fields', () => {
    const extra = applyAetherWSAccountConfig(
      {
        unrelated: 'keep',
        aether_ws: {
          future_option: 'keep'
        }
      },
      true
    )

    expect(extra).toMatchObject({
      unrelated: 'keep',
      openai_apikey_responses_websockets_v2_mode: 'passthrough',
      openai_apikey_responses_websockets_v2_enabled: true,
      aether_ws: {
        schema_version: AETHER_WS_SCHEMA_VERSION,
        enabled: true,
        required_control_protocol: AETHER_WS_CONTROL_PROTOCOL,
        future_option: 'keep'
      }
    })
  })

  it('atomically disables the account while preserving nested fields', () => {
    const extra = applyAetherWSAccountConfig(
      {
        aether_ws: {
          schema_version: 99,
          enabled: true,
          required_control_protocol: 'future',
          future_option: 42
        }
      },
      false
    )

    expect(extra).toMatchObject({
      openai_apikey_responses_websockets_v2_mode: 'off',
      openai_apikey_responses_websockets_v2_enabled: false,
      aether_ws: {
        schema_version: AETHER_WS_SCHEMA_VERSION,
        enabled: false,
        required_control_protocol: AETHER_WS_CONTROL_PROTOCOL,
        future_option: 42
      }
    })
  })

  it('reads configured state without treating malformed values as enabled', () => {
    expect(readAetherWSAccountState({
      openai_apikey_responses_websockets_v2_mode: 'passthrough',
      openai_apikey_responses_websockets_v2_enabled: true,
      aether_ws: {
        schema_version: 1,
        enabled: true,
        required_control_protocol: 'route-v1'
      }
    })).toEqual({
      configured: true,
      valid: true,
      enabled: true,
      reason: 'enabled'
    })
    expect(readAetherWSAccountState({ aether_ws: 'invalid' })).toEqual({
      configured: true,
      valid: false,
      enabled: false,
      reason: 'invalid_config'
    })
    expect(readAetherWSAccountState({
      openai_apikey_responses_websockets_v2_mode: 'off',
      openai_apikey_responses_websockets_v2_enabled: false,
      aether_ws: {
        schema_version: 1,
        enabled: true,
        required_control_protocol: 'route-v1'
      }
    })).toEqual({
      configured: true,
      valid: false,
      enabled: false,
      reason: 'field_conflict'
    })
  })
})
