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
import type { Table as TanstackTable } from '@tanstack/react-table'
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DISABLED_ROW_MOBILE } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  INVITATION_STATUS,
  INVITATION_STATUSES,
  REGISTER_METHODS,
} from '../constants'
import { isInvitationCodeExpired } from '../lib'
import type { InvitationCode } from '../types'
import { DataTableRowActions } from './data-table-row-actions'

const MOBILE_SKELETON_KEYS = [
  'invitation-mobile-skeleton-1',
  'invitation-mobile-skeleton-2',
  'invitation-mobile-skeleton-3',
  'invitation-mobile-skeleton-4',
  'invitation-mobile-skeleton-5',
]

function InvitationCodesMobileSkeleton() {
  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {MOBILE_SKELETON_KEYS.map((key) => (
        <div
          key={key}
          className='space-y-2 border-b px-3 py-2.5 last:border-b-0'
        >
          <div className='flex items-center justify-between'>
            <Skeleton className='h-4 w-32' />
            <Skeleton className='h-5 w-16 rounded-md' />
          </div>
          <div className='flex items-center justify-between gap-3'>
            <Skeleton className='h-7 w-44' />
            <Skeleton className='h-8 w-16' />
          </div>
          <Skeleton className='h-3 w-28' />
        </div>
      ))}
    </div>
  )
}

interface InvitationCodesMobileListProps {
  table: TanstackTable<InvitationCode>
  isLoading: boolean
}

export function InvitationCodesMobileList(
  props: InvitationCodesMobileListProps
) {
  const { t } = useTranslation()
  const rows = props.table.getRowModel().rows

  if (props.isLoading) return <InvitationCodesMobileSkeleton />

  if (!rows.length) {
    return (
      <div className='rounded-lg border p-8'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <KeyRound className='size-6' />
            </EmptyMedia>
            <EmptyTitle>{t('No Invitation Codes Found')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'No invitation codes available. Create your first invitation code to gate new registrations.'
              )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const invitationCode = row.original
        const expired = isInvitationCodeExpired(
          invitationCode.expired_time,
          invitationCode.status
        )
        const statusConfig = INVITATION_STATUSES[invitationCode.status]
        const methodConfig =
          REGISTER_METHODS[invitationCode.used_register_method ?? '']

        return (
          <div
            key={row.id}
            className={cn(
              'bg-card space-y-2.5 border-b px-3 py-2.5 last:border-b-0',
              expired || invitationCode.status !== INVITATION_STATUS.ENABLED
                ? DISABLED_ROW_MOBILE
                : undefined
            )}
          >
            <div className='flex items-start justify-between gap-3'>
              <div className='min-w-0'>
                <div className='truncate text-sm font-semibold'>
                  {invitationCode.name}
                </div>
                <div className='text-muted-foreground font-mono text-[11px]'>
                  {invitationCode.code_prefix}••••
                </div>
              </div>
              {expired ? (
                <StatusBadge
                  label={t('Expired')}
                  variant='warning'
                  copyable={false}
                />
              ) : (
                statusConfig && (
                  <StatusBadge
                    label={t(statusConfig.labelKey)}
                    variant={statusConfig.variant}
                    copyable={false}
                  />
                )
              )}
            </div>

            <div className='flex items-center justify-between gap-2'>
              <DataTableRowActions row={row} />
            </div>

            {invitationCode.used_user_id > 0 && (
              <div className='flex items-center justify-between gap-2 text-xs'>
                <span className='text-muted-foreground'>{t('Used By')}</span>
                <span className='font-medium'>
                  {invitationCode.used_username ||
                    t('User {{id}}', { id: invitationCode.used_user_id })}
                  {methodConfig ? ` · ${t(methodConfig.labelKey)}` : ''}
                </span>
              </div>
            )}

            {invitationCode.used_time > 0 && (
              <div className='flex items-center justify-between gap-2 text-xs'>
                <span className='text-muted-foreground'>{t('Used:')}</span>
                <span className='font-medium tabular-nums'>
                  {formatTimestampToDate(invitationCode.used_time)}
                </span>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
