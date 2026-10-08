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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { completeInvitationRegistration } from '@/features/auth/api'

import { InvitationComplete } from '../index'

const completeInvitationRegistrationMock = vi.mocked(
  completeInvitationRegistration
)
const handleLoginResultMock = vi.fn()

vi.mock('@/features/auth/api', () => ({
  completeInvitationRegistration: vi.fn(),
}))

vi.mock('../../hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({ handleLoginResult: handleLoginResultMock }),
}))

function renderComplete() {
  return render(
    <InvitationComplete flowToken='pending-flow-token' provider='github' />
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  cleanup()
})

it('keeps the form for retry when the invitation code is invalid', async () => {
  completeInvitationRegistrationMock.mockResolvedValue({
    success: false,
    message: 'Invitation code is invalid or has been used',
  })
  const user = userEvent.setup()

  renderComplete()
  await user.type(
    screen.getByLabelText(/invitation code/i),
    'INV-NOT-A-REAL-CODE'
  )
  await user.click(
    screen.getByRole('button', { name: /complete registration/i })
  )

  await waitFor(() => {
    expect(
      screen.getByText('Invitation code is invalid or has been used')
    ).toBeInTheDocument()
  })
  // The pending flow stays retryable: submit calls with the same token again.
  expect(completeInvitationRegistrationMock).toHaveBeenCalledWith(
    'pending-flow-token',
    'INV-NOT-A-REAL-CODE'
  )
})

it('completes the registration and signs the new account in with a valid code', async () => {
  completeInvitationRegistrationMock.mockResolvedValue({
    success: true,
    message: '',
    data: undefined,
  })
  handleLoginResultMock.mockResolvedValue(true)
  const user = userEvent.setup()

  renderComplete()
  await user.type(screen.getByLabelText(/invitation code/i), 'INV-VALID-1')
  await user.click(
    screen.getByRole('button', { name: /complete registration/i })
  )

  await waitFor(() => {
    expect(handleLoginResultMock).toHaveBeenCalled()
  })
  expect(completeInvitationRegistrationMock).toHaveBeenCalledWith(
    'pending-flow-token',
    'INV-VALID-1'
  )
})
