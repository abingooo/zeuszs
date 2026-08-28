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
import { useEffect, useRef, useState } from 'react'

import posterDarkAvif from '@/assets/home/orbital-earth-poster-dark.avif'
import posterDarkWebp from '@/assets/home/orbital-earth-poster-dark.webp'
import posterLightAvif from '@/assets/home/orbital-earth-poster-light.avif'
import posterLightWebp from '@/assets/home/orbital-earth-poster-light.webp'
import posterMobileDarkAvif from '@/assets/home/orbital-earth-poster-mobile-dark.avif'
import posterMobileDarkWebp from '@/assets/home/orbital-earth-poster-mobile-dark.webp'
import posterMobileLightAvif from '@/assets/home/orbital-earth-poster-mobile-light.avif'
import posterMobileLightWebp from '@/assets/home/orbital-earth-poster-mobile-light.webp'
import { useTheme } from '@/context/theme-provider'
import { cn } from '@/lib/utils'

import {
  clampHeroScrollProgress,
  getHeroSceneMode,
  getHeroScenePhase,
  type HeroSceneAppearance,
  type HeroSceneMode,
} from '../lib/hero-scene-policy'

interface NetworkInformationWithSaveData {
  saveData?: boolean
}

interface NavigatorWithPerformanceHints extends Navigator {
  connection?: NetworkInformationWithSaveData
  deviceMemory?: number
}

interface OrbitalEarthPosterProps {
  appearance: HeroSceneAppearance
  className?: string
  priority?: boolean
}

interface OrbitalEarthSceneProps {
  className?: string
}

export function OrbitalEarthPoster(props: OrbitalEarthPosterProps) {
  const desktopAvif =
    props.appearance === 'light' ? posterLightAvif : posterDarkAvif
  const desktopWebp =
    props.appearance === 'light' ? posterLightWebp : posterDarkWebp
  const mobileAvif =
    props.appearance === 'light' ? posterMobileLightAvif : posterMobileDarkAvif
  const mobileWebp =
    props.appearance === 'light' ? posterMobileLightWebp : posterMobileDarkWebp

  return (
    <picture
      aria-hidden='true'
      data-earth-poster-appearance={props.appearance}
      className={cn(
        'pointer-events-none absolute inset-0 block',
        props.className
      )}
    >
      <source
        media='(max-width: 767px)'
        srcSet={mobileAvif}
        type='image/avif'
      />
      <source
        media='(max-width: 767px)'
        srcSet={mobileWebp}
        type='image/webp'
      />
      <source srcSet={desktopAvif} type='image/avif' />
      <img
        src={desktopWebp}
        alt=''
        width={3840}
        height={2160}
        loading={props.priority === false ? 'lazy' : 'eager'}
        fetchPriority={props.priority === false ? 'auto' : 'high'}
        decoding='async'
        className='size-full object-cover object-[70%_68%] sm:object-center'
      />
    </picture>
  )
}

export function OrbitalEarthScene(props: OrbitalEarthSceneProps) {
  const { resolvedTheme } = useTheme()
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const progressRef = useRef(0)
  const appearanceRef = useRef<HeroSceneAppearance>(resolvedTheme)
  const [sceneMode, setSceneMode] = useState<HeroSceneMode>('static')
  const [sceneState, setSceneState] = useState<
    'fallback' | 'loading' | 'ready' | 'error'
  >('fallback')

  appearanceRef.current = resolvedTheme

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const reducedMotionQuery = window.matchMedia(
      '(prefers-reduced-motion: reduce)'
    )
    const navigatorWithHints = navigator as NavigatorWithPerformanceHints
    let webGLAvailable: boolean | undefined

    const resolveMode = () => {
      const capabilities = {
        reducedMotion: reducedMotionQuery.matches,
        saveData: navigatorWithHints.connection?.saveData === true,
        viewportWidth: window.innerWidth,
        deviceMemory: navigatorWithHints.deviceMemory,
        hardwareConcurrency: navigator.hardwareConcurrency,
      }

      if (
        getHeroSceneMode({ ...capabilities, webGLAvailable: true }) === 'static'
      ) {
        setSceneMode('static')
        return
      }

      if (webGLAvailable === undefined) {
        try {
          const capabilityCanvas = document.createElement('canvas')
          const context = capabilityCanvas.getContext('webgl2', {
            failIfMajorPerformanceCaveat: true,
          })
          webGLAvailable = context !== null
          context?.getExtension('WEBGL_lose_context')?.loseContext()
        } catch {
          webGLAvailable = false
        }
      }

      setSceneMode(getHeroSceneMode({ ...capabilities, webGLAvailable }))
    }

    resolveMode()
    reducedMotionQuery.addEventListener('change', resolveMode)
    window.addEventListener('resize', resolveMode, { passive: true })

    return () => {
      reducedMotionQuery.removeEventListener('change', resolveMode)
      window.removeEventListener('resize', resolveMode)
    }
  }, [])

  useEffect(() => {
    const container = containerRef.current
    const canvas = canvasRef.current
    if (!container || !canvas || sceneMode === 'static') {
      setSceneState('fallback')
      return
    }

    let cancelled = false
    let runtimeStopped = false
    let disposeRuntime: (() => void) | undefined
    let progressAnimationFrame: number | undefined
    let currentPhase = container.dataset.scenePhase
    const runtimeAbortController = new AbortController()

    const stopRuntime = () => {
      if (runtimeStopped) return
      runtimeStopped = true
      const cleanup = disposeRuntime
      disposeRuntime = undefined
      cleanup?.()
    }

    const updateProgress = () => {
      const rect = container.getBoundingClientRect()
      const nextProgress = clampHeroScrollProgress(
        -rect.top / Math.max(Math.min(rect.height * 0.38, 320), 1)
      )
      progressRef.current = nextProgress
      const nextPhase = getHeroScenePhase(nextProgress)
      if (nextPhase !== currentPhase) {
        currentPhase = nextPhase
        container.dataset.scenePhase = nextPhase
      }
    }

    const scheduleProgressUpdate = () => {
      if (progressAnimationFrame !== undefined) return
      progressAnimationFrame = window.requestAnimationFrame(() => {
        progressAnimationFrame = undefined
        updateProgress()
      })
    }

    setSceneState('loading')
    updateProgress()
    window.addEventListener('scroll', scheduleProgressUpdate, { passive: true })

    import('./orbital-earth-runtime')
      .then(async (runtime) => {
        if (cancelled || runtimeStopped) return
        const cleanup = await runtime.startOrbitalEarthRuntime({
          appearanceRef,
          canvas,
          container,
          progressRef,
          signal: runtimeAbortController.signal,
          onReady: () => {
            if (!cancelled && !runtimeStopped) setSceneState('ready')
          },
          onContextLost: () => {
            if (cancelled || runtimeStopped) return
            setSceneState('error')
            stopRuntime()
          },
        })

        if (cancelled || runtimeStopped) {
          cleanup()
          return
        }
        disposeRuntime = cleanup
      })
      .catch(() => {
        if (!cancelled) setSceneState('error')
      })

    return () => {
      cancelled = true
      runtimeAbortController.abort()
      window.removeEventListener('scroll', scheduleProgressUpdate)
      if (progressAnimationFrame !== undefined) {
        window.cancelAnimationFrame(progressAnimationFrame)
      }
      stopRuntime()
    }
  }, [sceneMode])

  return (
    <div
      ref={containerRef}
      aria-hidden='true'
      data-orbital-earth-scene
      data-scene-appearance={resolvedTheme}
      data-scene-mode={sceneMode}
      data-scene-state={sceneState}
      data-scene-phase='reveal'
      className={cn(
        'pointer-events-none absolute inset-0 overflow-hidden bg-[#edf3f7] transition-colors duration-500 dark:bg-[#121a27]',
        props.className
      )}
    >
      <OrbitalEarthPoster appearance={resolvedTheme} />
      <canvas
        ref={canvasRef}
        data-earth-scene
        className={cn(
          'absolute inset-0 size-full transition-opacity duration-700 ease-out',
          sceneState === 'ready' ? 'opacity-100' : 'opacity-0'
        )}
      />
    </div>
  )
}
