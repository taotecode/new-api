import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { register, wechatLoginByCode } from '@/features/auth/api'

import { SignUpForm } from '../sign-up-form'

const registerMock = vi.mocked(register)
const wechatLoginByCodeMock = vi.mocked(wechatLoginByCode)
const navigateMock = vi.fn()

vi.mock('@/features/auth/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/auth/api')>()),
  register: vi.fn(),
  wechatLoginByCode: vi.fn(),
}))

const toastErrorMock = vi.hoisted(() => vi.fn())
vi.mock('sonner', () => ({
  toast: { error: toastErrorMock, success: vi.fn() },
}))

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigateMock,
}))
vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    redirectToLogin: vi.fn(),
    handleLoginResult: vi.fn(),
  }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-email-verification', () => ({
  useEmailVerification: () => ({
    isSending: false,
    secondsLeft: 0,
    isActive: false,
    sendCode: vi.fn(),
  }),
}))

const statusRef = {
  status: null as Record<string, unknown> | null,
}

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: statusRef.status }),
}))

function renderSignUp(status: Record<string, unknown> | null) {
  statusRef.status = status
  return render(<SignUpForm />)
}

beforeEach(() => {
  vi.clearAllMocks()
  registerMock.mockResolvedValue({ success: true, message: '' })
})

afterEach(() => {
  cleanup()
})

const gateOffStatus = {
  register_enabled: true,
  password_register_enabled: true,
  invitation_required: false,
  invitation_methods: [],
}

const gateOnStatus = {
  register_enabled: true,
  password_register_enabled: true,
  invitation_required: true,
  invitation_methods: ['password'],
  user_agreement_enabled: false,
  privacy_policy_enabled: false,
}

async function fillPasswordFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/^username$/i), 'gated-user')
  await user.type(screen.getByLabelText(/^password$/i), 'password123')
  await user.type(screen.getByLabelText(/confirm password/i), 'password123')
}

it('hides the invitation input while the gate is off', async () => {
  renderSignUp(gateOffStatus)

  expect(screen.queryByLabelText(/invitation code/i)).not.toBeInTheDocument()
})

it('renders the gated invitation input last with a plain label', async () => {
  renderSignUp({ ...gateOnStatus, email_verification: true })

  expect(screen.getByLabelText(/^invitation code$/i)).toBeInTheDocument()
  expect(
    screen.queryByText(/invitation code \(required\)/i)
  ).not.toBeInTheDocument()

  const invitationInput = screen.getByLabelText(/^invitation code$/i)
  const verificationCodeInput =
    screen.getByPlaceholderText(/verification code/i)
  expect(
    verificationCodeInput.compareDocumentPosition(invitationInput) &
      Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
})

it('flags the empty required invitation code inline while gated', async () => {
  renderSignUp(gateOnStatus)
  const user = userEvent.setup()

  await fillPasswordFields(user)
  await user.click(screen.getByRole('button', { name: /create account/i }))

  const invitationInput = screen.getByLabelText(/invitation code/i)
  expect(invitationInput).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('Please enter an invitation code')).toBeVisible()
  expect(toastErrorMock).not.toHaveBeenCalledWith(
    'Please enter an invitation code'
  )
  expect(registerMock).not.toHaveBeenCalled()
})

it('flags the gated invitation code and required email inline when submitting an empty form', async () => {
  renderSignUp({ ...gateOnStatus, email_verification: true })
  const user = userEvent.setup()

  await user.click(screen.getByRole('button', { name: /create account/i }))

  expect(screen.getByLabelText(/invitation code/i)).toHaveAttribute(
    'aria-invalid',
    'true'
  )
  expect(screen.getByText('Please enter an invitation code')).toBeVisible()
  expect(
    screen.getByLabelText(/email \(required for verification\)/i)
  ).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('Please enter your email')).toBeVisible()
  expect(screen.getByText('Please enter your username')).toBeVisible()
  expect(registerMock).not.toHaveBeenCalled()
})

it('sends the invitation code with the password registration payload while gated', async () => {
  renderSignUp(gateOnStatus)
  const user = userEvent.setup()

  await fillPasswordFields(user)
  await user.type(screen.getByLabelText(/invitation code/i), 'INV-VALID-CODE')
  await user.click(screen.getByRole('button', { name: /create account/i }))

  await waitFor(() => {
    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        username: 'gated-user',
        invitation_code: 'INV-VALID-CODE',
      })
    )
  })
})

it('routes a gated WeChat identity to the invitation completion page', async () => {
  renderSignUp({
    ...gateOnStatus,
    wechat_login: true,
  })
  wechatLoginByCodeMock.mockResolvedValue({
    success: false,
    message: 'A valid invitation code is required to register',
    data: { invitation_required: true, flow_token: 'wechat-pending' },
  })
  const user = userEvent.setup()

  await user.click(
    screen.getByRole('button', { name: /continue with wechat/i })
  )
  const dialog = await screen.findByRole('dialog')
  await user.type(
    within(dialog).getByLabelText(/verification code/i),
    'wx-code'
  )
  await user.click(within(dialog).getByRole('button', { name: /^confirm$/i }))

  await waitFor(() => {
    expect(wechatLoginByCodeMock).toHaveBeenCalledWith('wx-code', undefined)
  })
  await waitFor(() => {
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/invitation-complete',
      search: { flow_token: 'wechat-pending', provider: 'wechat' },
    })
  })
})
