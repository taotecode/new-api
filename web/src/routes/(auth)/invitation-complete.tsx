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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { InvitationComplete } from '@/features/auth/invitation-complete'

const invitationCompleteSearchSchema = z.object({
  flow_token: z.string().optional().catch(''),
  provider: z.string().optional().catch(''),
})

function InvitationCompleteRoute() {
  const { flow_token: flowToken, provider } = Route.useSearch()
  if (!flowToken) {
    // Without a pending flow there is nothing to complete.
    throw redirect({ to: '/sign-up' })
  }
  return <InvitationComplete flowToken={flowToken} provider={provider || ''} />
}

export const Route = createFileRoute('/(auth)/invitation-complete')({
  validateSearch: invitationCompleteSearchSchema,
  component: InvitationCompleteRoute,
})
