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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useStatus } from '@/hooks/use-status'
import { cn } from '@/lib/utils'

import { HeroCapabilities } from '../hero-capabilities'
import { OrbitalBrandLockup } from '../orbital-brand-lockup'
import { OrbitalEarthPoster, OrbitalEarthScene } from '../orbital-earth-scene'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const HERO_FRAME_CLASSNAME =
  'relative isolate h-svh min-h-svh max-md:h-auto overflow-hidden bg-[#edf3f7] text-slate-950 transition-colors duration-500 dark:bg-[#121a27] dark:text-white'

export function HeroLoadingShell() {
  const { resolvedTheme } = useTheme()

  return (
    <section
      data-home-hero-loading
      data-hero-appearance={resolvedTheme}
      aria-busy='true'
      className={HERO_FRAME_CLASSNAME}
    >
      <OrbitalEarthPoster appearance={resolvedTheme} />
      <div className='relative z-10 mx-auto flex size-full max-w-7xl items-start px-6 pt-28 sm:px-8 sm:pt-36 lg:px-12 lg:pt-40'>
        <OrbitalBrandLockup className='text-slate-950 dark:text-[#fff8eb]' />
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

  const renderDocsButton = () => {
    const isExternal = docsUrl.startsWith('http')
    if (isExternal) {
      return (
        <Button
          variant='outline'
          className='group inline-flex h-11 items-center gap-1.5 rounded-lg border-slate-900/15 bg-white/45 px-5 text-sm font-medium text-slate-800 backdrop-blur-sm hover:border-slate-900/25 hover:bg-white/70 hover:text-slate-950 dark:border-white/25 dark:bg-white/5 dark:text-white dark:hover:border-white/40 dark:hover:bg-white/10 dark:hover:text-white'
          render={
            <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
          }
        >
          <BookOpen className='size-4 text-slate-600 transition-colors duration-200 group-hover:text-slate-950 dark:text-white/70 dark:group-hover:text-white' />
          <span>{t('Docs')}</span>
        </Button>
      )
    }
    return (
      <Button
        variant='outline'
        className='group inline-flex h-11 items-center gap-1.5 rounded-lg border-slate-900/15 bg-white/45 px-5 text-sm font-medium text-slate-800 backdrop-blur-sm hover:border-slate-900/25 hover:bg-white/70 hover:text-slate-950 dark:border-white/25 dark:bg-white/5 dark:text-white dark:hover:border-white/40 dark:hover:bg-white/10 dark:hover:text-white'
        render={<Link to={docsUrl} />}
      >
        <BookOpen className='size-4 text-slate-600 transition-colors duration-200 group-hover:text-slate-950 dark:text-white/70 dark:group-hover:text-white' />
        <span>{t('Docs')}</span>
      </Button>
    )
  }

  return (
    <section
      data-home-hero
      data-hero-appearance={resolvedTheme}
      className={cn(HERO_FRAME_CLASSNAME, props.className)}
    >
      <OrbitalEarthScene />

      <div className='relative z-10 mx-auto flex size-full max-w-7xl items-start px-6 pt-28 sm:px-8 sm:pt-36 lg:px-12 lg:pt-40'>
        <div className='flex max-w-xl flex-col items-start text-left lg:max-w-2xl'>
          <OrbitalBrandLockup className='landing-animate-fade-up text-slate-950 opacity-0 [animation-delay:120ms] dark:text-[#fff8eb]' />

          <div
            className='landing-animate-fade-up mt-9 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '240ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  className='group h-11 rounded-lg bg-slate-950 px-5 text-sm font-medium text-white shadow-[0_10px_35px_rgba(15,23,42,0.12)] hover:bg-slate-800 dark:bg-white dark:text-black dark:shadow-[0_10px_35px_rgba(255,255,255,0.12)] dark:hover:bg-white/90'
                  render={<Link to='/dashboard' />}
                >
                  {t('Go to Dashboard')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                {renderDocsButton()}
              </>
            ) : (
              <>
                <Button
                  className='group h-11 rounded-lg bg-slate-950 px-5 text-sm font-medium text-white shadow-[0_10px_35px_rgba(15,23,42,0.12)] hover:bg-slate-800 dark:bg-white dark:text-black dark:shadow-[0_10px_35px_rgba(255,255,255,0.12)] dark:hover:bg-white/90'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                {renderDocsButton()}
              </>
            )}
          </div>

          <p
            className='landing-animate-fade-up mt-5 max-w-lg text-sm leading-6 text-slate-600 opacity-0 sm:text-[15px] dark:text-white/42'
            style={{ animationDelay: '360ms' }}
          >
            {t(
              'Supports one-click configuration and perfectly adapts to NewAPI multi-protocol configuration.'
            )}
          </p>

          <HeroCapabilities />
        </div>
      </div>
    </section>
  )
}
