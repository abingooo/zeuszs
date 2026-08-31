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
import { act, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import type React from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zhCN from '@/i18n/locales/zh.json'

import { Hero, HeroLoadingShell } from '../sections/hero'

const { startOrbitalEarthRuntimeMock, themeState } = vi.hoisted(() => ({
  startOrbitalEarthRuntimeMock: vi.fn(),
  themeState: { resolvedTheme: 'light' as 'dark' | 'light' },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, ...props }: React.ComponentProps<'a'> & { to: string }) => (
    <a href={to} {...props} />
  ),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { docs_link: 'https://docs.zeuszs.example' },
  }),
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => themeState,
}))

vi.mock('../orbital-earth-runtime', () => ({
  startOrbitalEarthRuntime: startOrbitalEarthRuntimeMock,
}))

function createMediaQueryList(query: string, matches = false): MediaQueryList {
  return {
    matches,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }
}

function mockWebGLAvailable(): void {
  vi.mocked(HTMLCanvasElement.prototype.getContext).mockImplementation(
    (contextId) => {
      if (contextId !== 'webgl2') return null
      return {
        getExtension: () => ({ loseContext: () => undefined }),
      } as unknown as WebGL2RenderingContext
    }
  )
}

describe('orbital Earth hero', () => {
  beforeEach(async () => {
    i18next.addResourceBundle('en', 'translation', en.translation, true, true)
    i18next.addResourceBundle(
      'zhCN',
      'translation',
      zhCN.translation,
      true,
      true
    )
    await i18next.changeLanguage('en')

    vi.spyOn(window, 'matchMedia').mockImplementation(createMediaQueryList)
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(
      () => null
    )
    themeState.resolvedTheme = 'light'
    startOrbitalEarthRuntimeMock.mockReset()
    startOrbitalEarthRuntimeMock.mockResolvedValue(() => undefined)
  })

  afterEach(async () => {
    vi.restoreAllMocks()
    await i18next.changeLanguage('en')
    i18next.removeResourceBundle('en', 'translation')
    i18next.removeResourceBundle('zhCN', 'translation')
  })

  test('presents ZEUSZS as the English page heading', () => {
    const { container } = render(<Hero isAuthenticated={false} />)
    const hero = container.querySelector('[data-home-hero]')

    expect(
      screen.getByRole('heading', { level: 1, name: 'ZEUSZS' })
    ).toBeVisible()
    expect(container.querySelector('[data-home-hero]')).toHaveClass('min-h-svh')
    expect(container.querySelector('[data-home-hero]')).not.toHaveClass('h-svh')
    expect(container.querySelector('[data-home-hero]')).not.toHaveClass(
      'h-[calc(100svh-4rem)]'
    )
    expect(hero).not.toHaveTextContent(
      'Supports one-click configuration and perfectly adapts to NewAPI multi-protocol configuration.'
    )
    expect(hero).not.toHaveTextContent('AI infrastructure for teams')
  })

  test('fills the hero information panel with localized platform capabilities', () => {
    const { container } = render(<Hero isAuthenticated={false} />)

    const capabilities = container.querySelector('[data-home-capabilities]')
    expect(capabilities).toBeVisible()
    expect(capabilities).toHaveTextContent(
      'A unified AI model platform for enterprise R&D and research teams'
    )
    expect(capabilities).not.toHaveTextContent('AI infrastructure for teams')
    expect(capabilities).toHaveTextContent('Unified API')
    expect(capabilities).toHaveTextContent('Team controls')
    expect(capabilities).toHaveTextContent('Clear billing')
    expect(capabilities).toHaveTextContent('Protected access')
    expect(
      capabilities?.querySelectorAll('[data-home-capability]')
    ).toHaveLength(4)
    expect(capabilities?.querySelectorAll('[data-home-workflow]')).toHaveLength(
      4
    )
  })

  test('keeps the capability heading wide and readable across line wraps', () => {
    const { container } = render(<Hero isAuthenticated={false} />)

    const heading = container.querySelector('[data-home-capabilities] h2')

    expect(heading).toHaveClass('text-pretty')
    expect(heading?.parentElement).toHaveClass('max-w-2xl')
  })

  test('updates the page heading to 宙斯智算 after switching to Simplified Chinese', async () => {
    render(<Hero isAuthenticated={false} />)

    await act(async () => {
      await i18next.changeLanguage('zhCN')
    })

    expect(
      screen.getByRole('heading', { level: 1, name: '宙斯智算' })
    ).toBeVisible()
  })

  test('localizes the capability panel after switching to Simplified Chinese', async () => {
    render(<Hero isAuthenticated={false} />)

    await act(async () => {
      await i18next.changeLanguage('zhCN')
    })

    const capabilities = document.querySelector('[data-home-capabilities]')
    expect(capabilities).toHaveTextContent(
      '面向企业研发与科研团队的统一 AI 模型平台'
    )
    expect(capabilities).not.toHaveTextContent('面向团队的 AI 基础设施')
    expect(capabilities).toHaveTextContent('统一 API')
    expect(capabilities).toHaveTextContent('文献研读')
    expect(document.querySelector('[data-home-hero]')).not.toHaveTextContent(
      '支持一键配置并完美适配 NewAPI 多协议配置'
    )
  })

  test('uses the same original-logo brand lockup in the loading shell', () => {
    const { container } = render(<HeroLoadingShell />)

    const loadingHero = container.querySelector('[data-home-hero-loading]')
    expect(loadingHero).toContainElement(
      container.querySelector('[data-orbital-brand-lockup]')
    )
    expect(loadingHero).toHaveClass('min-h-svh')
    expect(loadingHero).not.toHaveClass('h-svh')
    expect(loadingHero).not.toHaveClass('h-[calc(100svh-4rem)]')
    expect(
      screen.getByRole('heading', { level: 1, name: 'ZEUSZS' })
    ).toBeVisible()
  })

  test.each(['light', 'dark'] as const)(
    'provides an immediate $appearance poster matching the hero appearance',
    (appearance) => {
      themeState.resolvedTheme = appearance
      const { container } = render(<Hero isAuthenticated={false} />)

      const hero = container.querySelector('[data-home-hero]')
      const scene = container.querySelector('[data-orbital-earth-scene]')
      const picture = scene?.querySelector('picture')
      const poster = picture?.querySelector('img')
      const sources = [...(picture?.querySelectorAll('source') ?? [])]

      expect(hero).toHaveAttribute('data-hero-appearance', appearance)
      expect(scene).toHaveAttribute('aria-hidden', 'true')
      expect(scene).toHaveAttribute('data-scene-appearance', appearance)
      expect(picture).toHaveAttribute('aria-hidden', 'true')
      expect(picture).toHaveAttribute(
        'data-earth-poster-appearance',
        appearance
      )
      expect(sources).toHaveLength(3)
      for (const source of sources) {
        expect(source.getAttribute('srcset')).toContain(appearance)
      }
      expect(poster?.getAttribute('src')).toContain(appearance)
      expect(poster).toHaveAttribute('alt', '')
      expect(poster).toHaveAttribute('loading', 'eager')
      expect(poster).toHaveAttribute('width', '3840')
      expect(poster).toHaveAttribute('height', '2160')
    }
  )

  test('keeps the canvas hidden and the static poster active when WebGL is unavailable', async () => {
    const { container } = render(<Hero isAuthenticated={false} />)

    const scene = container.querySelector('[data-orbital-earth-scene]')
    const canvas = container.querySelector('canvas[data-earth-scene]')

    await waitFor(() => {
      expect(scene).toHaveAttribute('data-scene-mode', 'static')
      expect(scene).toHaveAttribute('data-scene-state', 'fallback')
    })
    expect(canvas).toHaveClass('opacity-0')
    expect(scene?.querySelector('picture img')).toBeInTheDocument()
  })

  test('uses the static poster when reduced motion is requested despite WebGL support', async () => {
    vi.mocked(window.matchMedia).mockImplementation((query) =>
      createMediaQueryList(query, query === '(prefers-reduced-motion: reduce)')
    )
    mockWebGLAvailable()

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')

    await waitFor(() => {
      expect(scene).toHaveAttribute('data-scene-mode', 'static')
    })
    expect(HTMLCanvasElement.prototype.getContext).not.toHaveBeenCalled()
    expect(startOrbitalEarthRuntimeMock).not.toHaveBeenCalled()
    expect(scene?.querySelector('picture img')).toBeInTheDocument()
  })

  test('uses the mobile poster without probing WebGL on a narrow viewport', async () => {
    vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(390)
    mockWebGLAvailable()

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')
    const mobileSources = scene?.querySelectorAll(
      'source[media="(max-width: 767px)"]'
    )

    await waitFor(() => {
      expect(scene).toHaveAttribute('data-scene-mode', 'static')
    })
    expect(mobileSources).toHaveLength(2)
    expect(HTMLCanvasElement.prototype.getContext).not.toHaveBeenCalled()
    expect(startOrbitalEarthRuntimeMock).not.toHaveBeenCalled()
    expect(scene?.querySelector('canvas')).toHaveClass('opacity-0')
  })

  test('updates the public scene phase as the interactive hero is scrolled', async () => {
    mockWebGLAvailable()

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector<HTMLElement>(
      '[data-orbital-earth-scene]'
    )
    expect(scene).not.toBeNull()

    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
      expect(scene).toHaveAttribute('data-scene-mode', 'interactive')
    })

    vi.spyOn(scene as HTMLElement, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: -250,
      top: -250,
      right: 1440,
      bottom: 550,
      left: 0,
      width: 1440,
      height: 800,
      toJSON: () => ({}),
    })
    window.dispatchEvent(new Event('scroll'))

    await waitFor(() => {
      expect(scene).toHaveAttribute('data-scene-phase', 'network')
    })
  })

  test('shows the interactive canvas after the orbital runtime reports ready', async () => {
    mockWebGLAvailable()

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')
    const canvas = container.querySelector('canvas[data-earth-scene]')

    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
    })
    const runtimeOptions = startOrbitalEarthRuntimeMock.mock.calls[0][0]
    act(() => runtimeOptions.onReady())

    expect(scene).toHaveAttribute('data-scene-state', 'ready')
    expect(canvas).toHaveClass('opacity-100')
    expect(scene?.querySelector('picture img')).toBeInTheDocument()
  })

  test('aborts the orbital runtime signal when the hero unmounts', async () => {
    mockWebGLAvailable()

    const { unmount } = render(<Hero isAuthenticated={false} />)

    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
    })
    const runtimeOptions = startOrbitalEarthRuntimeMock.mock.calls[0][0]
    expect(runtimeOptions.signal).toBeInstanceOf(AbortSignal)
    expect(runtimeOptions.signal.aborted).toBe(false)

    unmount()

    expect(runtimeOptions.signal.aborted).toBe(true)
  })

  test('cleans up a late runtime without restoring ready after the scene falls back', async () => {
    mockWebGLAvailable()
    const viewportWidth = vi
      .spyOn(window, 'innerWidth', 'get')
      .mockReturnValue(1440)
    let resolveRuntime: ((cleanup: () => void) => void) | undefined
    const pendingRuntime = new Promise<() => void>((resolve) => {
      resolveRuntime = resolve
    })
    startOrbitalEarthRuntimeMock.mockReturnValueOnce(pendingRuntime)

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')
    const canvas = container.querySelector('canvas[data-earth-scene]')

    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
      expect(scene).toHaveAttribute('data-scene-state', 'loading')
    })
    const runtimeOptions = startOrbitalEarthRuntimeMock.mock.calls[0][0]

    viewportWidth.mockReturnValue(767)
    window.dispatchEvent(new Event('resize'))

    await waitFor(() => {
      expect(runtimeOptions.signal.aborted).toBe(true)
      expect(scene).toHaveAttribute('data-scene-mode', 'static')
      expect(scene).toHaveAttribute('data-scene-state', 'fallback')
    })

    act(() => runtimeOptions.onReady())
    expect(scene).toHaveAttribute('data-scene-state', 'fallback')
    expect(canvas).toHaveClass('opacity-0')

    const cleanup = vi.fn()
    await act(async () => {
      resolveRuntime?.(cleanup)
      await pendingRuntime
    })

    expect(cleanup).toHaveBeenCalledOnce()
    expect(scene).toHaveAttribute('data-scene-state', 'fallback')
    expect(canvas).toHaveClass('opacity-0')
  })

  test('updates the orbital appearance without restarting the runtime', async () => {
    mockWebGLAvailable()
    const cleanup = vi.fn()
    startOrbitalEarthRuntimeMock.mockResolvedValueOnce(cleanup)

    const { container, rerender } = render(<Hero isAuthenticated={false} />)
    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
    })
    const runtimeOptions = startOrbitalEarthRuntimeMock.mock.calls[0][0]
    const canvasBeforeThemeChange = container.querySelector(
      'canvas[data-earth-scene]'
    )
    act(() => runtimeOptions.onReady())
    expect(
      container.querySelector('[data-orbital-earth-scene]')
    ).toHaveAttribute('data-scene-state', 'ready')

    themeState.resolvedTheme = 'dark'
    rerender(<Hero isAuthenticated={false} />)

    const hero = container.querySelector('[data-home-hero]')
    const scene = container.querySelector('[data-orbital-earth-scene]')
    expect(hero).toHaveAttribute('data-hero-appearance', 'dark')
    expect(scene).toHaveAttribute('data-scene-appearance', 'dark')
    expect(scene?.querySelector('picture')).toHaveAttribute(
      'data-earth-poster-appearance',
      'dark'
    )
    expect(runtimeOptions.appearanceRef.current).toBe('dark')
    expect(container.querySelector('canvas[data-earth-scene]')).toBe(
      canvasBeforeThemeChange
    )
    expect(scene).toHaveAttribute('data-scene-state', 'ready')
    expect(runtimeOptions.signal.aborted).toBe(false)
    expect(cleanup).not.toHaveBeenCalled()
    expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
  })

  test('stops the interactive runtime and restores the poster after WebGL context loss', async () => {
    mockWebGLAvailable()
    const cleanup = vi.fn()
    startOrbitalEarthRuntimeMock.mockResolvedValueOnce(cleanup)

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')
    const canvas = container.querySelector('canvas[data-earth-scene]')

    await waitFor(() => {
      expect(startOrbitalEarthRuntimeMock).toHaveBeenCalledOnce()
    })
    const runtimeOptions = startOrbitalEarthRuntimeMock.mock.calls[0][0]
    await act(async () => runtimeOptions.onContextLost())

    expect(cleanup).toHaveBeenCalledOnce()
    expect(scene).toHaveAttribute('data-scene-state', 'error')
    expect(canvas).toHaveClass('opacity-0')
    expect(scene?.querySelector('picture img')).toBeInTheDocument()
  })

  test('keeps the poster visible when the interactive runtime fails to start', async () => {
    mockWebGLAvailable()
    startOrbitalEarthRuntimeMock.mockRejectedValueOnce(
      new Error('runtime unavailable')
    )

    const { container } = render(<Hero isAuthenticated={false} />)
    const scene = container.querySelector('[data-orbital-earth-scene]')
    const canvas = container.querySelector('canvas[data-earth-scene]')

    await waitFor(() => {
      expect(scene).toHaveAttribute('data-scene-mode', 'interactive')
      expect(scene).toHaveAttribute('data-scene-state', 'error')
    })
    expect(canvas).toHaveClass('opacity-0')
    expect(scene?.querySelector('picture img')).toBeInTheDocument()
  })

  test('shows visitor actions without rendering the retired terminal demo', () => {
    const { container } = render(<Hero isAuthenticated={false} />)

    expect(screen.getByRole('button', { name: 'Get Started' })).toHaveAttribute(
      'href',
      '/sign-up'
    )
    expect(screen.getByRole('button', { name: 'Docs' })).toHaveAttribute(
      'href',
      'https://docs.zeuszs.example'
    )
    expect(screen.queryByText('/v1/chat/completions')).not.toBeInTheDocument()
    expect(screen.queryByText(/200 ok/i)).not.toBeInTheDocument()
    expect(container.querySelector('[data-home-hero]')).not.toHaveTextContent(
      /storm|lightning/i
    )
  })
})
