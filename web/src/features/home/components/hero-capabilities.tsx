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
  Analytics01Icon,
  ApiGatewayIcon,
  ReceiptTextIcon,
  Route01Icon,
  SecurityIcon,
  UserGroupIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

type CapabilityTone = 'blue' | 'green' | 'amber' | 'cyan' | 'indigo' | 'rose'

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
    title: string
  }
> = {
  blue: {
    icon: 'text-sky-700 dark:text-sky-300',
    iconFrame:
      'border-sky-500/25 bg-sky-500/10 group-hover/capability:border-sky-500/40 group-hover/capability:bg-sky-500/15',
    accent: 'bg-sky-500/70',
    title: 'text-sky-700 dark:text-sky-300',
  },
  green: {
    icon: 'text-emerald-700 dark:text-emerald-300',
    iconFrame:
      'border-emerald-500/25 bg-emerald-500/10 group-hover/capability:border-emerald-500/40 group-hover/capability:bg-emerald-500/15',
    accent: 'bg-emerald-500/70',
    title: 'text-emerald-700 dark:text-emerald-300',
  },
  amber: {
    icon: 'text-amber-700 dark:text-amber-300',
    iconFrame:
      'border-amber-500/25 bg-amber-500/10 group-hover/capability:border-amber-500/40 group-hover/capability:bg-amber-500/15',
    accent: 'bg-amber-500/70',
    title: 'text-amber-700 dark:text-amber-300',
  },
  cyan: {
    icon: 'text-cyan-700 dark:text-cyan-300',
    iconFrame:
      'border-cyan-500/25 bg-cyan-500/10 group-hover/capability:border-cyan-500/40 group-hover/capability:bg-cyan-500/15',
    accent: 'bg-cyan-500/70',
    title: 'text-cyan-700 dark:text-cyan-300',
  },
  indigo: {
    icon: 'text-indigo-700 dark:text-indigo-300',
    iconFrame:
      'border-indigo-500/25 bg-indigo-500/10 group-hover/capability:border-indigo-500/40 group-hover/capability:bg-indigo-500/15',
    accent: 'bg-indigo-500/70',
    title: 'text-indigo-700 dark:text-indigo-300',
  },
  rose: {
    icon: 'text-rose-700 dark:text-rose-300',
    iconFrame:
      'border-rose-500/25 bg-rose-500/10 group-hover/capability:border-rose-500/40 group-hover/capability:bg-rose-500/15',
    accent: 'bg-rose-500/70',
    title: 'text-rose-700 dark:text-rose-300',
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
    id: 'routing',
    title: 'Intelligent routing',
    description:
      'Match channels by model and group, then route by priority and weight',
    icon: Route01Icon,
    tone: 'indigo',
  },
  {
    id: 'teams',
    title: 'Team controls',
    description: 'Members, roles, and usage in one place',
    icon: UserGroupIcon,
    tone: 'green',
  },
  {
    id: 'insights',
    title: 'Usage insights',
    description: 'Clear views of requests, tokens, and cost trends',
    icon: Analytics01Icon,
    tone: 'rose',
  },
  {
    id: 'billing',
    title: 'Clear billing',
    description: 'Quota billing with corporate invoices',
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

const USE_CASES = [
  {
    id: 'agents',
    label: 'Intelligent agents',
    marker: 'bg-sky-500/75',
    text: 'text-sky-700 dark:text-sky-300',
  },
  {
    id: 'code',
    label: 'Code development',
    marker: 'bg-emerald-500/75',
    text: 'text-emerald-700 dark:text-emerald-300',
  },
  {
    id: 'writing',
    label: 'Content writing',
    marker: 'bg-rose-500/75',
    text: 'text-rose-700 dark:text-rose-300',
  },
  {
    id: 'documents',
    label: 'Document organization',
    marker: 'bg-amber-500/75',
    text: 'text-amber-700 dark:text-amber-300',
  },
  {
    id: 'analysis',
    label: 'Data analysis',
    marker: 'bg-cyan-500/75',
    text: 'text-cyan-700 dark:text-cyan-300',
  },
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
      aria-label={t('Platform capabilities')}
      data-home-capabilities
      className='landing-animate-fade-up mx-auto w-full max-w-5xl min-w-0 opacity-0 [animation-delay:240ms]'
    >
      <ul
        aria-label={t('Platform capabilities')}
        data-home-capability-grid
        className='grid auto-rows-fr grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3'
      >
        {capabilities.map((capability) => {
          const tone = CAPABILITY_TONES[capability.tone]

          return (
            <li
              key={capability.id}
              data-home-capability={capability.id}
              data-home-glass-tile
              className='group/capability border-border/70 bg-background/80 hover:border-border hover:bg-background/95 supports-[backdrop-filter]:bg-background/60 relative flex min-h-28 min-w-0 items-center gap-4 overflow-hidden rounded-lg border p-4 shadow-[0_18px_50px_-34px_rgba(15,23,42,0.5)] backdrop-blur-lg transition-[transform,border-color,background-color,box-shadow] duration-300 hover:shadow-[0_24px_58px_-30px_rgba(15,23,42,0.55)] motion-safe:hover:-translate-y-1 motion-reduce:transform-none motion-reduce:transition-none sm:h-full sm:p-5'
            >
              <span
                className={cn(
                  'bg-background/65 relative z-10 flex size-10 shrink-0 items-center justify-center rounded-lg border shadow-sm transition-[transform,border-color,background-color,box-shadow] duration-200 group-hover/capability:shadow-md motion-reduce:transform-none motion-reduce:transition-none motion-safe:group-hover/capability:scale-105',
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

              <div className='relative z-10 min-w-0 flex-1'>
                <h3
                  className={cn(
                    'text-base leading-tight font-semibold tracking-normal break-words sm:text-lg',
                    tone.title
                  )}
                >
                  {capability.title}
                </h3>
                <p className='text-muted-foreground mt-1 text-sm leading-6 break-words sm:text-[15px]'>
                  {capability.description}
                </p>
              </div>

              <span
                aria-hidden='true'
                className={cn(
                  'absolute top-4 left-0 h-8 w-0.5 rounded-full opacity-70 transition-[height,opacity] duration-200 group-hover/capability:h-11 group-hover/capability:opacity-100 motion-reduce:transition-none sm:top-5',
                  tone.accent
                )}
              />
            </li>
          )
        })}
      </ul>

      <div data-home-use-cases className='border-border/60 mt-6 border-t pt-5'>
        <ul
          aria-label={t('Application scenarios')}
          data-home-use-case-list
          className='flex flex-wrap items-center justify-center gap-x-5 gap-y-2 text-sm leading-5 sm:text-[15px]'
        >
          {USE_CASES.map((useCase) => (
            <li
              key={useCase.id}
              data-home-use-case={useCase.id}
              className={cn(
                'inline-flex items-center gap-1.5 font-medium',
                useCase.text
              )}
            >
              <span
                aria-hidden='true'
                className={cn('size-1.5 rounded-full', useCase.marker)}
              />
              {t(useCase.label)}
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}
