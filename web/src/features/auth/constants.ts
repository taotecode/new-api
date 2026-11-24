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
import { z } from 'zod'

import { accountPasswordSchema } from '@/lib/password-policy'

// ============================================================================
// Form Schemas
// ============================================================================

export const loginFormSchema = z.object({
  username: z.string().min(1, 'Please enter your username or email'),
  password: z.string().min(1, 'Please enter your password'),
})

type RegisterSchemaOptions = {
  // The invitation gate applies to the password entry.
  requireInvitationCode?: boolean
  // The site requires email verification for registration.
  requireEmail?: boolean
}

export function buildRegisterSchema(options: RegisterSchemaOptions = {}) {
  const { requireInvitationCode = false, requireEmail = false } = options
  return z
    .object({
      username: z.string().min(1, 'Please enter your username'),
      email: z.string().optional(),
      password: accountPasswordSchema,
      confirmPassword: z.string().min(1, 'Please confirm your password'),
      invitationCode: z.string().optional(),
    })
    .refine((data) => data.password === data.confirmPassword, {
      message: "Passwords don't match.",
      path: ['confirmPassword'],
    })
    .superRefine((data, ctx) => {
      if (requireInvitationCode && !data.invitationCode?.trim()) {
        ctx.addIssue({
          code: 'custom',
          path: ['invitationCode'],
          message: 'Please enter an invitation code',
        })
      }
      if (requireEmail && !data.email?.trim()) {
        ctx.addIssue({
          code: 'custom',
          path: ['email'],
          message: 'Please enter your email',
        })
      }
    })
}

export const registerFormSchema = buildRegisterSchema()

export const forgotPasswordFormSchema = z.object({
  email: z.string().email({
    message: 'Please enter a valid email address',
  }),
})

export const otpFormSchema = z.object({
  otp: z.string().min(1, 'Please enter a code.'),
})

// ============================================================================
// Validation Constants
// ============================================================================

export const PASSWORD_MIN_LENGTH = 8
export const OTP_LENGTH = 6
export const BACKUP_CODE_LENGTH = 9 // XXXX-XXXX format
export const BACKUP_CODE_REGEX = /^[A-Z0-9]{4}-[A-Z0-9]{4}$/i
export const OTP_REGEX = /^\d{6}$/

// ============================================================================
// Countdown Constants
// ============================================================================

export const EMAIL_VERIFICATION_COUNTDOWN = 30 // seconds
export const PASSWORD_RESET_COUNTDOWN = 30 // seconds

// ============================================================================
// OAuth Constants
// ============================================================================

export const OAUTH_POPUP_CALLBACK_MESSAGE = 'oauth:popup:callback'
export const OAUTH_POPUP_RESULT_MESSAGE = 'oauth:popup:result'
export const TELEGRAM_BIND_RESULT_MESSAGE = 'telegram:binding:result'
