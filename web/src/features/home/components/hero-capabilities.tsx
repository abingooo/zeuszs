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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type CapabilityTone = 'blue' | 'green' | 'amber' | 'cyan'

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
  cyan: {
    icon: 'text-cyan-700 dark:text-cyan-300',
    iconFrame:
      'border-cyan-500/25 bg-cyan-500/10 group-hover:border-cyan-500/40 group-hover:bg-cyan-500/15',
    accent: 'bg-cyan-500/70',
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
    tone: 'cyan',
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

type HeroCapabilitiesProps = {
  actions?: ReactNode
}

export function HeroCapabilities(props: HeroCapabilitiesProps) {
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
      className='landing-animate-fade-up mt-7 w-full max-w-2xl opacity-0 max-sm:-mx-3 max-sm:rounded-2xl max-sm:bg-white/60 max-sm:p-3 max-sm:backdrop-blur-[2px] dark:max-sm:bg-[#121a27]/70'
      style={{ animationDelay: '240ms' }}
    >
      <div className='max-w-2xl'>
        <h2
          id='home-capabilities-title'
          className='text-[1.65rem] leading-[1.15] font-semibold tracking-normal text-pretty text-slate-950 sm:text-[2rem] dark:text-white'
        >
          {t(
            'A unified AI model platform for enterprise R&D and research teams'
          )}
        </h2>
        <p className='mt-3 max-w-lg text-sm leading-6 text-slate-700/85 sm:text-[15px] dark:text-white/65'>
          {t(
            'Connect leading models through one API, then manage members, keys, usage, and spend in one place.'
          )}
        </p>
      </div>

      {props.actions ? (
        <div className='mt-6 flex flex-wrap items-center gap-3'>
          {props.actions}
        </div>
      ) : null}

      <ul
        aria-label={t('Platform capabilities')}
        className='mt-6 grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2'
      >
        {capabilities.map((capability) => {
          const tone = CAPABILITY_TONES[capability.tone]

          return (
            <li
              key={capability.id}
              data-home-capability={capability.id}
              className='group relative flex min-h-[4.5rem] min-w-0 items-start gap-2.5 border-t border-slate-900/10 pt-3 transition-colors duration-300 dark:border-white/15'
            >
              <div className='flex min-w-0 shrink-0 items-center gap-2.5'>
                <span
                  className={cn(
                    'flex size-8 shrink-0 items-center justify-center rounded-lg border bg-white/35 transition-colors duration-300 dark:bg-white/[0.04]',
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
              </div>
              <div className='min-w-0 pt-0.5'>
                <h3 className='line-clamp-2 text-[13px] leading-tight font-semibold tracking-normal text-slate-900 sm:text-sm dark:text-white'>
                  {capability.title}
                </h3>
                <p className='mt-1 line-clamp-2 text-[11px] leading-4 text-slate-600 sm:text-xs dark:text-white/55'>
                  {capability.description}
                </p>
              </div>
              <span
                aria-hidden='true'
                className={cn(
                  'absolute left-0 top-3 h-8 w-0.5 rounded-full opacity-70 transition-opacity duration-300 group-hover:opacity-100',
                  tone.accent
                )}
              />
            </li>
          )
        })}
      </ul>

      <div
        data-home-workflows
        className='mt-5 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-slate-600/90 dark:text-white/55'
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
