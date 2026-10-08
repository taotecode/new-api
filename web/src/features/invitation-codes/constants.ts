/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TFunction } from 'i18next'

import type { StatusBadgeProps } from '@/components/status-badge'

// ============================================================================
// Invitation Status Configuration
// ============================================================================

export const INVITATION_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  USED: 3,
} as const

export const INVITATION_STATUS_VALUES = Object.values(INVITATION_STATUS).map(
  (value) => String(value)
) as `${number}`[]

// labelKey values are i18n keys; use t(config.labelKey) in components
export const INVITATION_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant'> & {
    labelKey: string
    value: number
  }
> = {
  [INVITATION_STATUS.ENABLED]: {
    labelKey: 'Unused',
    variant: 'success',
    value: INVITATION_STATUS.ENABLED,
  },
  [INVITATION_STATUS.DISABLED]: {
    labelKey: 'Disabled',
    variant: 'neutral',
    value: INVITATION_STATUS.DISABLED,
  },
  [INVITATION_STATUS.USED]: {
    labelKey: 'Used',
    variant: 'neutral',
    value: INVITATION_STATUS.USED,
  },
} as const

// Virtual status filter value for expired invitation codes
// Note: "Expired" is not a real DB status, it's computed from expired_time
export const INVITATION_FILTER_EXPIRED = 'expired'

export const INVITATION_FILTER_VALUES = [
  String(INVITATION_STATUS.ENABLED),
  String(INVITATION_STATUS.DISABLED),
  String(INVITATION_STATUS.USED),
  INVITATION_FILTER_EXPIRED,
] as const

export function getInvitationStatusOptions(t: TFunction) {
  return [
    ...Object.values(INVITATION_STATUSES).map((config) => ({
      label: t(config.labelKey),
      value: String(config.value),
    })),
    {
      label: t('Expired'),
      value: INVITATION_FILTER_EXPIRED,
    },
  ]
}

// ============================================================================
// Registration Methods
// ============================================================================

// labelKey values are i18n keys; use t(config.labelKey) in components
export const REGISTER_METHODS: Record<string, { labelKey: string }> = {
  password: { labelKey: 'Password' },
  github: { labelKey: 'GitHub' },
  discord: { labelKey: 'Discord' },
  oidc: { labelKey: 'OIDC' },
  linuxdo: { labelKey: 'LinuxDO' },
  custom_oauth: { labelKey: 'Custom OAuth' },
  wechat: { labelKey: 'WeChat' },
  admin_create: { labelKey: 'Admin Created' },
}

// ============================================================================
// Validation Constants
// ============================================================================

export const INVITATION_VALIDATION = {
  NAME_MIN_LENGTH: 1,
  NAME_MAX_LENGTH: 20,
  COUNT_MIN: 1,
  COUNT_MAX: 100,
} as const

// ============================================================================
// Error Messages
// ============================================================================

// i18n keys; use t(ERROR_MESSAGES.xxx) when displaying. For form schema with interpolation use getInvitationFormErrorMessages(t).
export const ERROR_MESSAGES = {
  UNEXPECTED: 'An unexpected error occurred',
  LOAD_FAILED: 'Failed to load invitation codes',
  SEARCH_FAILED: 'Failed to search invitation codes',
  CREATE_FAILED: 'Failed to create invitation codes',
  UPDATE_FAILED: 'Failed to update invitation code',
  DELETE_FAILED: 'Failed to delete invitation code',
  DELETE_USED_FAILED: 'Failed to delete used invitation codes',
  STATUS_UPDATE_FAILED: 'Failed to update invitation code status',
  NAME_LENGTH_INVALID: 'Name must be between {{min}} and {{max}} characters',
  COUNT_INVALID: 'Count must be between {{min}} and {{max}}',
  EXPIRED_TIME_INVALID: 'Expired time cannot be earlier than current time',
} as const

/** For form schema only: returns translated messages with interpolation. */
export function getInvitationFormErrorMessages(t: TFunction) {
  return {
    NAME_LENGTH_INVALID: t(ERROR_MESSAGES.NAME_LENGTH_INVALID, {
      min: INVITATION_VALIDATION.NAME_MIN_LENGTH,
      max: INVITATION_VALIDATION.NAME_MAX_LENGTH,
    }),
    COUNT_INVALID: t(ERROR_MESSAGES.COUNT_INVALID, {
      min: INVITATION_VALIDATION.COUNT_MIN,
      max: INVITATION_VALIDATION.COUNT_MAX,
    }),
    EXPIRED_TIME_INVALID: t(ERROR_MESSAGES.EXPIRED_TIME_INVALID),
  } as const
}

// ============================================================================
// Success Messages (i18n keys; use t(SUCCESS_MESSAGES.xxx) when displaying)
// ============================================================================

export const SUCCESS_MESSAGES = {
  INVITATION_CREATED: 'Invitation code(s) created successfully',
  INVITATION_UPDATED: 'Invitation code updated successfully',
  INVITATION_DELETED: 'Invitation code deleted successfully',
  INVITATION_ENABLED: 'Invitation code enabled successfully',
  INVITATION_DISABLED: 'Invitation code disabled successfully',
} as const
