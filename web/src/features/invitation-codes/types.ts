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

// ============================================================================
// Invitation Code Schema & Types
// ============================================================================

export const invitationCodeSchema = z.object({
  id: z.number(),
  name: z.string(),
  // Only the display prefix is stored server-side; the full plaintext code is
  // returned exactly once at creation and never again.
  code_prefix: z.string(),
  status: z.number(), // 1: enabled, 2: disabled, 3: used
  state: z.string().optional(), // enabled | disabled | used | expired (derived)
  created_by: z.number(),
  used_user_id: z.number(),
  used_username: z.string().optional(),
  used_register_method: z.string().optional(),
  created_by_username: z.string().optional(),
  created_time: z.number(),
  used_time: z.number(),
  expired_time: z.number(), // 0 for never expires
})

export type InvitationCode = z.infer<typeof invitationCodeSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetInvitationCodesParams {
  p?: number
  page_size?: number
}

export interface GetInvitationCodesResponse {
  success: boolean
  message?: string
  data?: {
    items: InvitationCode[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchInvitationCodesParams {
  keyword?: string
  status?: string
  p?: number
  page_size?: number
}

export interface InvitationCodeFormData {
  id?: number
  name: string
  expired_time: number
  count?: number // Only for create
  status?: number // Only for status update
}

// ============================================================================
// Dialog Types
// ============================================================================

export type InvitationCodesDialogType = 'create' | 'update' | 'delete' | 'view'
