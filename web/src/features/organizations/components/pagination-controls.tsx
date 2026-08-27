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
import { ArrowLeft01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type PaginationControlsProps = {
  page: number
  pageSize: number
  total: number
  disabled?: boolean
  onPageChange: (page: number) => void
}

export function PaginationControls(props: PaginationControlsProps) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))

  return (
    <div className='flex min-w-0 items-center justify-between gap-3 border-t pt-3'>
      <p className='text-muted-foreground truncate text-xs sm:text-sm'>
        {t('{{total}} total', { total: props.total })}
      </p>
      <div className='flex shrink-0 items-center gap-2'>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          disabled={props.disabled || props.page <= 1}
          onClick={() => props.onPageChange(props.page - 1)}
          aria-label={t('Previous page')}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
        </Button>
        <span className='min-w-16 text-center text-xs font-medium tabular-nums sm:text-sm'>
          {t('{{page}} / {{pageCount}}', {
            page: props.page,
            pageCount,
          })}
        </span>
        <Button
          type='button'
          variant='outline'
          size='icon-sm'
          disabled={props.disabled || props.page >= pageCount}
          onClick={() => props.onPageChange(props.page + 1)}
          aria-label={t('Next page')}
        >
          <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
        </Button>
      </div>
    </div>
  )
}
