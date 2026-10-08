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
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import { deleteUsedInvitationCodes } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { useInvitationCodes } from './invitation-codes-provider'

export function InvitationCodesPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useInvitationCodes()
  const [showDeleteUsedConfirm, setShowDeleteUsedConfirm] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDeleteUsed = async () => {
    setIsDeleting(true)
    try {
      const result = await deleteUsedInvitationCodes()
      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} used invitation codes', {
            count,
          })
        )
        triggerRefresh()
        setShowDeleteUsedConfirm(false)
      } else {
        handleServerError(result, t(ERROR_MESSAGES.DELETE_USED_FAILED))
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <div className='flex flex-wrap gap-2'>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setShowDeleteUsedConfirm(true)}
        >
          <Trash2 className='text-destructive h-4 w-4' />
          {t('Delete Used')}
        </Button>
        <Button size='sm' onClick={() => setOpen('create')}>
          <Plus className='h-4 w-4' />
          {t('Create Code')}
        </Button>
      </div>

      <ConfirmDialog
        destructive
        open={showDeleteUsedConfirm}
        onOpenChange={setShowDeleteUsedConfirm}
        handleConfirm={handleDeleteUsed}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete Used Invitation Codes?')}
        desc={
          <>
            <Trans
              t={t}
              i18nKey='This will delete all <strong>used</strong> invitation codes. Their consumption records will be removed with them.'
              components={{ strong: <strong /> }}
            />
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Used')}
      />
    </>
  )
}
