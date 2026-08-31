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
*/
import {
  ApiGatewayIcon,
  ReceiptTextIcon,
  SecurityIcon,
  UserGroupIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type CapabilityTone = 'blue' | 'green' | 'amber' | 'violet'

type Capability = {
  id: string
  title: string
  description: string
  icon: IconSvgElement
  tone: CapabilityTone
}

const CAPABILITY_TONES: Record<
  CapabilityTone,
  {
    icon: string
    iconFrame: string
    accent: string
  }
> = {
  blue: {
    icon: 'text-sky-700 dark:text-sky-300',
    iconFrame:
      'border-sky-500/25 bg-sky-500/10 group-hover:border-sky-500/40 group-hover:bg-sky-500/15',
    accent: 'bg-sky-500/70',
  },
  green: {
    icon: 'text-emerald-700 dark:text-emerald-300',
    iconFrame:
      'border-emerald-500/25 bg-emerald-500/10 group-hover:border-emerald-500/40 group-hover:bg-emerald-500/15',
    accent: 'bg-emerald-500/70',
  },
  amber: {
    icon: 'text-amber-700 dark:text-amber-300',
    iconFrame:
      'border-amber-500/25 bg-amber-500/10 group-hover:border-amber-500/40 group-hover:bg-amber-500/15',
    accent: 'bg-amber-500/70',
  },
  violet: {
    icon: 'text-violet-700 dark:text-violet-300',
    iconFrame:
      'border-violet-500/25 bg-violet-500/10 group-hover:border-violet-500/40 group-hover:bg-violet-500/15',
    accent: 'bg-violet-500/70',
  },
}

const CAPABILITY_DEFINITIONS = [
  {
    id: 'api',
    title: 'Unified API',
    description: 'One endpoint for leading models',
    icon: ApiGatewayIcon,
    tone: 'blue',
  },
  {
    id: 'teams',
    title: 'Team controls',
    description: 'Members, roles, and usage in one place',
    icon: UserGroupIcon,
    tone: 'green',
  },
  {
    id: 'billing',
    title: 'Clear billing',
    description: 'Quota-aware RMB billing',
    icon: ReceiptTextIcon,
    tone: 'amber',
  },
  {
    id: 'security',
    title: 'Protected access',
    description: 'Scoped keys and secure access',
    icon: SecurityIcon,
    tone: 'violet',
  },
] satisfies ReadonlyArray<{
  id: string
  title: string
  description: string
  icon: IconSvgElement
  tone: CapabilityTone
}>

const WORKFLOW_KEYS = [
  'Literature review',
  'Code and data',
  'Paper writing',
  'Team collaboration',
] as const

export function HeroCapabilities() {
  const { t } = useTranslation()

  const capabilities: Capability[] = CAPABILITY_DEFINITIONS.map((item) => ({
    ...item,
    title: t(item.title),
    description: t(item.description),
  }))

  return (
    <section
      aria-labelledby='home-capabilities-title'
      data-home-capabilities
      className='landing-animate-fade-up mt-5 w-full max-w-2xl opacity-0 max-sm:-mx-3 max-sm:rounded-2xl max-sm:bg-white/65 max-sm:p-3 max-sm:backdrop-blur-[2px] dark:max-sm:bg-[#121a27]/70'
      style={{ animationDelay: '420ms' }}
    >
      <div className='max-w-xl'>
        <p className='text-sm font-semibold tracking-[0.12em] text-slate-700/80 uppercase dark:text-white/65'>
          {t('AI infrastructure for teams')}
        </p>
        <h2
          id='home-capabilities-title'
          className='mt-2 text-xl leading-tight font-semibold tracking-tight text-slate-950 sm:text-2xl dark:text-white'
        >
          {t(
            'A unified AI model platform for enterprise R&D and research teams'
          )}
        </h2>
        <p className='mt-2 max-w-lg text-sm leading-6 text-slate-700/85 sm:text-[15px] dark:text-white/65'>
          {t(
            'Connect leading models through one API, then manage members, keys, usage, and spend in one place.'
          )}
        </p>
      </div>

      <ul
        aria-label={t('Platform capabilities')}
        className='mt-4 grid grid-cols-2 gap-2.5 max-[374px]:grid-cols-1 sm:gap-3'
      >
        {capabilities.map((capability) => {
          const tone = CAPABILITY_TONES[capability.tone]

          return (
            <li
              key={capability.id}
              data-home-capability={capability.id}
              className='group relative flex h-[5.75rem] min-h-0 flex-col justify-between overflow-hidden rounded-xl border border-slate-900/10 bg-white/45 p-3 backdrop-blur-[2px] transition-colors duration-300 hover:bg-white/65 max-sm:bg-white/80 sm:h-24 sm:p-3.5 dark:border-white/15 dark:bg-white/[0.045] dark:hover:bg-white/[0.08] dark:max-sm:bg-white/[0.11]'
            >
              <div className='flex min-w-0 items-center gap-2.5'>
                <span
                  className={cn(
                    'flex size-8 shrink-0 items-center justify-center rounded-lg border transition-colors duration-300',
                    tone.iconFrame
                  )}
                >
                  <HugeiconsIcon
                    icon={capability.icon}
                    aria-hidden='true'
                    className={cn('size-[1.1rem]', tone.icon)}
                    strokeWidth={1.8}
                  />
                </span>
                <h3 className='line-clamp-2 min-w-0 text-[13px] leading-tight font-semibold text-slate-900 sm:text-sm dark:text-white'>
                  {capability.title}
                </h3>
              </div>
              <p className='mt-2 line-clamp-2 text-[11px] leading-4 text-slate-600 sm:text-xs dark:text-white/55'>
                {capability.description}
              </p>
              <span
                aria-hidden='true'
                className={cn(
                  'absolute inset-x-0 bottom-0 h-px opacity-0 transition-opacity duration-300 group-hover:opacity-100',
                  tone.accent
                )}
              />
            </li>
          )
        })}
      </ul>

      <div
        data-home-workflows
        className='mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-slate-600/90 dark:text-white/55'
      >
        <span className='font-medium text-slate-800 dark:text-white/75'>
          {t('Built for')}
        </span>
        {WORKFLOW_KEYS.map((workflow, index) => (
          <span
            key={workflow}
            data-home-workflow={index}
            className='inline-flex items-center gap-1.5'
          >
            <span
              aria-hidden='true'
              className={cn(
                'size-1.5 rounded-full',
                index % 2 === 0 ? 'bg-sky-500/80' : 'bg-amber-500/80'
              )}
            />
            {t(workflow)}
          </span>
        ))}
      </div>
    </section>
  )
}
