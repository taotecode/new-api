import { zodResolver } from '@hookform/resolvers/zod'
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
import { KeyRound, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { completeInvitationRegistration } from '@/features/auth/api'
import { useAuthRedirect } from '@/features/auth/hooks/use-auth-redirect'
import { AuthOperationError } from '@/lib/secure-verification'

const invitationCompleteSchema = z.object({
  invitationCode: z
    .string()
    .min(1, 'Please enter an invitation code')
    .max(128, 'Invitation code is too long'),
})

type InvitationCompleteFormValues = z.infer<typeof invitationCompleteSchema>

type InvitationCompleteProps = {
  flowToken: string
  provider: string
}

export function InvitationComplete(props: InvitationCompleteProps) {
  const { t } = useTranslation()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const { handleLoginResult } = useAuthRedirect()

  const form = useForm<InvitationCompleteFormValues>({
    resolver: zodResolver(invitationCompleteSchema),
    defaultValues: { invitationCode: '' },
  })

  const onSubmit = async (data: InvitationCompleteFormValues) => {
    if (isSubmitting) return
    setIsSubmitting(true)
    try {
      const res = await completeInvitationRegistration(
        props.flowToken,
        data.invitationCode.trim()
      )
      if (res?.success) {
        if (await handleLoginResult(res.data)) {
          toast.success(t('Account created successfully!'))
        }
      } else {
        // Keep the flow alive: an invalid code can be retried while the
        // pending identity has not expired.
        form.setError('invitationCode', {
          message: getServerMessage(res, t),
        })
      }
    } catch (error: unknown) {
      // Network or rejected requests (for example an expired or already
      // consumed flow) surface as form errors so the user can restart
      // instead of hitting an unhandled rejection.
      const normalized = AuthOperationError.from(
        error,
        t('Invitation code is invalid or has been used')
      )
      form.setError('invitationCode', { message: normalized.message })
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className='mx-auto w-full max-w-sm space-y-6'>
      <div className='flex flex-col items-center gap-2 text-center'>
        <div className='bg-primary/10 text-primary flex size-12 items-center justify-center rounded-full'>
          <KeyRound className='size-6' aria-hidden='true' />
        </div>
        <h1 className='text-2xl font-semibold tracking-tight'>
          {t('Complete registration')}
        </h1>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Your account is one step away. Enter a valid invitation code or a referral code to finish creating it.'
          )}
        </p>
      </div>

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className='grid gap-4'
          noValidate
        >
          <FormField
            control={form.control}
            name='invitationCode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Invitation code')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Enter invitation code')}
                    autoComplete='off'
                    autoFocus
                    disabled={isSubmitting}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'The invitation code is required by the site administrator. Contact the inviter if you do not have one.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type='submit'
            className='w-full justify-center gap-2'
            disabled={isSubmitting}
          >
            {isSubmitting ? <Loader2 className='h-4 w-4 animate-spin' /> : null}
            {t('Complete registration')}
          </Button>
        </form>
      </Form>

      <p className='text-muted-foreground text-center text-xs'>
        {t(
          'The authorized identity is kept for 10 minutes. Without a valid code, no account is created.'
        )}
      </p>
    </div>
  )
}

function getServerMessage(
  res: { success: boolean; message?: string },
  fallback: (key: string) => string
): string {
  return res.message || fallback('Invitation code is invalid or has been used')
}
