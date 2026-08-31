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

type OrbitalBrandLockupProps = {
  className?: string
}

export function OrbitalBrandLockup(props: OrbitalBrandLockupProps) {
  const { i18n, t } = useTranslation()
  const { logo, systemName, loading } = useSystemConfig()
  const language = normalizeInterfaceLanguage(
    i18n.resolvedLanguage || i18n.language
  )
  const isChinese = language === 'zhCN'
  const displayName = loading ? t('ZEUSZS') : systemName
  const displayLogo = loading ? '/zeuszs-logo.png' : logo || '/zeuszs-logo.png'

  return (
    <h1
      data-orbital-brand-lockup
      data-brand-language={language}
      lang={isChinese ? 'zh-CN' : 'en'}
      className={cn(
        'text-foreground inline-flex max-w-full items-center gap-3 text-left sm:gap-4 lg:gap-5',
        props.className
      )}
    >
      <img
        data-zeuszs-logo
        src={displayLogo}
        alt=''
        aria-hidden='true'
        width='512'
        height='512'
        draggable='false'
        className='size-14 shrink-0 object-contain sm:size-[4.5rem] lg:size-20'
      />
      <span
        data-brand-wordmark={language}
        className='font-inter block min-w-0 shrink text-[2.5rem] leading-none font-semibold tracking-normal whitespace-nowrap sm:text-[3.5rem] lg:text-[4.25rem]'
      >
        {displayName}
      </span>
    </h1>
  )
}
