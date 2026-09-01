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
import { useTranslation } from 'react-i18next'

import { useSystemConfig } from '@/hooks/use-system-config'
import { normalizeInterfaceLanguage } from '@/i18n/languages'
import { cn } from '@/lib/utils'

type HeroBrandLockupProps = {
  className?: string
}

export function HeroBrandLockup(props: HeroBrandLockupProps) {
  const { i18n, t } = useTranslation()
  const { systemName, loading } = useSystemConfig()
  const language = normalizeInterfaceLanguage(
    i18n.resolvedLanguage || i18n.language
  )
  const isChinese = language === 'zhCN'
  const displayName = loading ? t('ZEUSZS') : systemName

  return (
    <h1
      data-hero-brand-lockup
      data-brand-language={language}
      lang={isChinese ? 'zh-CN' : 'en'}
      className={cn(
        'text-foreground inline-flex max-w-full items-center gap-3 text-left sm:gap-4 lg:gap-5',
        props.className
      )}
    >
      <img
        data-zeuszs-logo
        src='/zeuszs-logo.png'
        alt=''
        aria-hidden='true'
        width='512'
        height='512'
        draggable='false'
        className='size-14 shrink-0 object-contain sm:size-[4.75rem] lg:size-[5.5rem]'
      />
      <span
        data-brand-wordmark={language}
        className='font-inter block min-w-0 shrink text-[2.75rem] leading-none font-semibold tracking-normal text-balance [overflow-wrap:anywhere] sm:text-[3.75rem] lg:text-[4.5rem]'
      >
        {displayName}
      </span>
    </h1>
  )
}
