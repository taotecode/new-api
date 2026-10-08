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
import { zodResolver } from '@hookform/resolvers/zod'
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

// Registration entries whose account creation the invitation gate can cover.
const INVITATION_METHOD_OPTIONS = [
  { value: 'password', labelKey: 'Password' },
  { value: 'github', labelKey: 'GitHub' },
  { value: 'discord', labelKey: 'Discord' },
  { value: 'oidc', labelKey: 'OIDC' },
  { value: 'linuxdo', labelKey: 'LinuxDO' },
  { value: 'custom_oauth', labelKey: 'Custom OAuth' },
  { value: 'wechat', labelKey: 'WeChat' },
] as const

const invitationSchema = z.object({
  InvitationRequired: z.boolean(),
  InvitationAffAllowed: z.boolean(),
  InvitationMethods: z.string(),
})

type InvitationFormValues = z.infer<typeof invitationSchema>

type InvitationSectionProps = {
  defaultValues: InvitationFormValues
}

export function InvitationSection({ defaultValues }: InvitationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<InvitationFormValues>({
    resolver: zodResolver(invitationSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const methodsValue = form.watch('InvitationMethods')
  const selectedMethods = useMemo(
    () =>
      new Set(
        methodsValue
          .split(',')
          .map((method) => method.trim())
          .filter(Boolean)
      ),
    [methodsValue]
  )

  const toggleMethod = (method: string, checked: boolean) => {
    const next = new Set(selectedMethods)
    if (checked) {
      next.add(method)
    } else {
      next.delete(method)
    }
    const ordered = INVITATION_METHOD_OPTIONS.filter((option) =>
      next.has(option.value)
    ).map((option) => option.value)
    form.setValue('InvitationMethods', ordered.join(','), {
      shouldDirty: true,
    })
  }

  const onSubmit = async (data: InvitationFormValues) => {
    const updates: Array<{ key: string; value: string | boolean }> = []

    if (data.InvitationRequired !== defaultValues.InvitationRequired) {
      updates.push({
        key: 'InvitationRequired',
        value: data.InvitationRequired,
      })
    }
    if (data.InvitationAffAllowed !== defaultValues.InvitationAffAllowed) {
      updates.push({
        key: 'InvitationAffAllowed',
        value: data.InvitationAffAllowed,
      })
    }
    if (data.InvitationMethods !== defaultValues.InvitationMethods) {
      updates.push({
        key: 'InvitationMethods',
        value: data.InvitationMethods,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Invitation Code Registration')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='InvitationRequired'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Require Invitation Code')}</FormLabel>
                  <FormDescription>
                    {t(
                      'New accounts must present a valid invitation code or a referral code before they are created. Existing users are unaffected.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='InvitationAffAllowed'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Accept Referral Codes')}</FormLabel>
                  <FormDescription>
                    {t(
                      "Existing users' referral codes can pass the invitation gate and bind the inviter."
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='InvitationMethods'
            render={() => (
              <FormItem>
                <FormLabel>{t('Gated Registration Entries')}</FormLabel>
                <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
                  {INVITATION_METHOD_OPTIONS.map((option) => {
                    const inputId = `invitation-method-${option.value}`
                    return (
                      <div
                        key={option.value}
                        className='flex items-center gap-2'
                      >
                        <Checkbox
                          id={inputId}
                          checked={selectedMethods.has(option.value)}
                          onCheckedChange={(checked) =>
                            toggleMethod(option.value, !!checked)
                          }
                        />
                        <Label htmlFor={inputId}>{t(option.labelKey)}</Label>
                      </div>
                    )
                  })}
                </div>
                <FormDescription>
                  {t(
                    'Registration entries covered by the invitation gate. OAuth entries pause after authorization until a valid code is provided.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormItem>
            <FormLabel>{t('Invitation Codes')}</FormLabel>
            <FormDescription>
              {t(
                'Create and manage one-time invitation codes for new registrations.'
              )}
            </FormDescription>
            <Button
              variant='outline'
              size='sm'
              render={<Link to='/invitation-codes' />}
            >
              {t('Open Invitation Codes')}
            </Button>
          </FormItem>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
