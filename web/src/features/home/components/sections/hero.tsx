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
import {
  ArrowRight01Icon,
  BookOpen01Icon,
  Tag01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'

import { HeroBrandLockup } from '../hero-brand-lockup'
import { HeroCapabilities } from '../hero-capabilities'
import { SpaceBackground } from '../space-background'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const HERO_FRAME_CLASSNAME =
  'relative isolate min-h-svh overflow-hidden bg-[#edf3f7] text-slate-950 transition-colors duration-500 dark:bg-[#121a27] dark:text-white'

const HERO_LAYOUT_CLASSNAME =
  'relative z-10 mx-auto flex min-h-svh w-full max-w-7xl flex-col px-6 pt-24 pb-6 sm:px-8 sm:pt-28 sm:pb-8 lg:px-12 lg:pt-24'

const HERO_CONTENT_CLASSNAME =
  'flex w-full flex-1 flex-col items-center justify-center gap-8 sm:gap-10'

const HERO_PRIMARY_ACTION_CLASSNAME =
  'group h-11 w-full rounded-lg bg-sky-600 px-5 text-base font-medium text-white shadow-[0_10px_35px_rgba(2,132,199,0.18)] transition-[transform,background-color,box-shadow] duration-200 hover:bg-sky-700 hover:shadow-[0_14px_40px_rgba(2,132,199,0.24)] motion-safe:hover:-translate-y-0.5 motion-reduce:transform-none motion-reduce:transition-none sm:w-auto dark:bg-sky-400 dark:text-slate-950 dark:shadow-[0_10px_35px_rgba(56,189,248,0.16)] dark:hover:bg-sky-300'

const HERO_SECONDARY_ACTION_CLASSNAME =
  'group inline-flex h-11 w-full items-center justify-center rounded-lg border-slate-900/15 bg-white/45 px-4 text-sm font-medium text-slate-800 shadow-sm backdrop-blur-md transition-[transform,border-color,background-color,box-shadow,color] duration-200 hover:border-slate-900/25 hover:bg-white/70 hover:text-slate-950 hover:shadow-md motion-safe:hover:-translate-y-0.5 motion-reduce:transform-none motion-reduce:transition-none sm:w-auto sm:px-5 sm:text-base dark:border-white/25 dark:bg-white/5 dark:text-white dark:hover:border-white/40 dark:hover:bg-white/10 dark:hover:text-white'

export function HeroLoadingShell() {
  const { resolvedTheme } = useTheme()

  return (
    <section
      data-home-hero-loading
      data-hero-appearance={resolvedTheme}
      aria-busy='true'
      className={HERO_FRAME_CLASSNAME}
    >
      <SpaceBackground />
      <div className={HERO_LAYOUT_CLASSNAME}>
        <div className={HERO_CONTENT_CLASSNAME}>
          <HeroBrandLockup className='text-slate-950 dark:text-[#fff8eb]' />
        </div>
      </div>
    </section>
  )
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const { status } = useStatus()
  const docsUrl =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'
  const docsButtonRender = docsUrl.startsWith('http') ? (
    <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
  ) : (
    <Link to={docsUrl} />
  )
  const primaryRoute = props.isAuthenticated ? '/dashboard' : '/sign-up'

  return (
    <section
      data-home-hero
      data-hero-appearance={resolvedTheme}
      className={cn(HERO_FRAME_CLASSNAME, props.className)}
    >
      <SpaceBackground />

      <div data-home-hero-layout className={HERO_LAYOUT_CLASSNAME}>
        <div data-home-hero-content className={HERO_CONTENT_CLASSNAME}>
          <div
            data-home-hero-intro
            className='flex w-full max-w-6xl min-w-0 flex-col items-center text-center'
          >
            <HeroBrandLockup className='landing-animate-fade-up justify-center text-center text-slate-950 opacity-0 [animation-delay:120ms] dark:text-[#fff8eb]' />

            <div
              data-home-hero-copy
              className='landing-animate-fade-up mt-6 w-full max-w-6xl opacity-0 [animation-delay:200ms]'
            >
              <h2 className='text-foreground mx-auto w-full text-2xl leading-[1.2] font-semibold tracking-normal sm:text-3xl sm:leading-[1.18] lg:text-[2rem]'>
                {t(
                  'A unified AI model platform for enterprise developers and research teams'
                )}
              </h2>

              <div
                data-home-value-statement
                className='mx-auto mt-4 flex max-w-4xl flex-wrap items-center justify-center gap-x-3 gap-y-1 text-lg leading-7 font-semibold tracking-normal sm:text-xl sm:leading-8'
              >
                <span
                  data-home-value-phrase='product'
                  className='text-foreground/75 w-full sm:w-auto'
                >
                  <Trans
                    i18nKey='Bring more focus back to <product>products</product>'
                    components={{
                      product: (
                        <strong
                          data-home-value-focus='product'
                          className='font-semibold text-emerald-700 dark:text-emerald-300'
                        />
                      ),
                    }}
                  />
                </span>
                <span
                  data-home-value-separator
                  aria-hidden='true'
                  className='text-muted-foreground/45 hidden sm:inline'
                >
                  ·
                </span>
                <span
                  data-home-value-phrase='research'
                  className='text-foreground/75 w-full sm:w-auto'
                >
                  <Trans
                    i18nKey='Put more energy into <research>research</research>'
                    components={{
                      research: (
                        <strong
                          data-home-value-focus='research'
                          className='font-semibold text-amber-700 dark:text-amber-300'
                        />
                      ),
                    }}
                  />
                </span>
              </div>

              <p
                data-home-hero-description
                className='text-muted-foreground mx-auto mt-3 max-w-3xl text-[15px] leading-6 sm:text-base'
              >
                {t(
                  'Connect leading models through one API, then manage members, keys, usage, and spend in one place.'
                )}
              </p>

              <div
                data-home-actions
                className='mx-auto mt-6 grid w-full max-w-56 grid-cols-1 items-center gap-3 sm:max-w-xl sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]'
              >
                <Button
                  data-home-action='docs'
                  variant='outline'
                  className={cn(
                    HERO_SECONDARY_ACTION_CLASSNAME,
                    'sm:justify-self-end'
                  )}
                  render={docsButtonRender}
                >
                  <HugeiconsIcon
                    icon={BookOpen01Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  <span>{t('Integration Docs')}</span>
                </Button>

                <Button
                  data-home-action='primary'
                  className={HERO_PRIMARY_ACTION_CLASSNAME}
                  render={<Link to={primaryRoute} />}
                >
                  {props.isAuthenticated
                    ? t('Go to Dashboard')
                    : t('Get Started')}
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    data-icon='inline-end'
                    aria-hidden='true'
                    className='transition-transform duration-200 motion-safe:group-hover:translate-x-0.5 motion-reduce:transition-none'
                  />
                </Button>

                <Button
                  data-home-action='pricing'
                  variant='outline'
                  className={cn(
                    HERO_SECONDARY_ACTION_CLASSNAME,
                    'sm:justify-self-start'
                  )}
                  render={<Link to='/pricing' />}
                >
                  <HugeiconsIcon
                    icon={Tag01Icon}
                    data-icon='inline-start'
                    aria-hidden='true'
                  />
                  <span>{t('Model Pricing')}</span>
                </Button>
              </div>
            </div>
          </div>

          <HeroCapabilities />
        </div>

        <footer
          data-home-company-footer
          className='border-border/50 mx-auto mt-8 w-full max-w-5xl border-t pt-5 text-center'
        >
          <p className='text-muted-foreground text-[13px] leading-5 sm:text-sm'>
            &copy; {new Date().getFullYear()}{' '}
            <span lang='zh-CN'>宙斯智算（上海）科技有限公司</span>{' '}
            {t('All rights reserved')}
          </p>
        </footer>
      </div>
    </section>
  )
}
