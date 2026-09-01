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
import { act, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type React from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zhCN from '@/i18n/locales/zh.json'

import { Hero, HeroLoadingShell } from '../sections/hero'

const themeState = { resolvedTheme: 'light' as 'dark' | 'light' }

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, ...props }: React.ComponentProps<'a'> & { to: string }) => (
    <a href={to} {...props} />
  ),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      docs_link: 'https://docs.zeuszs.example',
      api_info_enabled: true,
      api_info: [{ url: '   ' }, { url: '  https://api.zeuszs.ai///  ' }],
      server_address: 'https://zeuszs.example',
    },
  }),
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => themeState,
}))

describe('home hero', () => {
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
    themeState.resolvedTheme = 'light'
  })

  afterEach(async () => {
    vi.useRealTimers()
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
    expect(hero).toHaveClass('min-h-svh')
    expect(hero).not.toHaveClass('h-svh')
    expect(hero).not.toHaveClass('h-[calc(100svh-4rem)]')
    expect(hero).not.toHaveTextContent(
      'Supports one-click configuration and perfectly adapts to NewAPI multi-protocol configuration.'
    )
    expect(hero).not.toHaveTextContent('AI infrastructure for teams')
  })

  test('fills the information panel with localized platform capabilities', () => {
    const { container } = render(<Hero isAuthenticated={false} />)

    const heroCopy = container.querySelector('[data-home-hero-copy]')
    const capabilities = container.querySelector('[data-home-capabilities]')
    expect(capabilities).toBeVisible()
    expect(heroCopy).toHaveTextContent(
      'A unified AI model platform for enterprise developers and research teams'
    )
    expect(capabilities).not.toHaveTextContent('AI infrastructure for teams')
    expect(capabilities).toHaveTextContent('Unified API')
    expect(capabilities).toHaveTextContent('Intelligent routing')
    expect(capabilities).toHaveTextContent('Team controls')
    expect(capabilities).toHaveTextContent('Usage insights')
    expect(capabilities).toHaveTextContent('Clear billing')
    expect(capabilities).toHaveTextContent(
      'Quota billing with corporate invoices'
    )
    expect(capabilities).not.toHaveTextContent('Quota-aware RMB billing')
    expect(capabilities).toHaveTextContent('Protected access')
    expect(capabilities).not.toHaveTextContent('Built for')
    expect(
      screen.getByRole('list', { name: 'Application scenarios' })
    ).toBeVisible()
    expect(
      [...(capabilities?.querySelectorAll('[data-home-capability]') ?? [])].map(
        (item) => item.getAttribute('data-home-capability')
      )
    ).toEqual(['api', 'routing', 'teams', 'insights', 'billing', 'security'])
    expect(
      [...(capabilities?.querySelectorAll('[data-home-use-case]') ?? [])].map(
        (item) => item.textContent?.trim()
      )
    ).toEqual([
      'Intelligent agents',
      'Code development',
      'Content writing',
      'Document organization',
      'Data analysis',
    ])
    expect(
      capabilities?.querySelectorAll('[data-home-capability]')
    ).toHaveLength(6)
    expect(capabilities?.querySelectorAll('[data-home-use-case]')).toHaveLength(
      5
    )
    expect(
      capabilities?.querySelector('[data-home-use-case-list]')
    ).toHaveClass('flex', 'flex-wrap', 'justify-center')
    expect(
      capabilities?.querySelector('[data-home-audience]')
    ).not.toBeInTheDocument()
    expect(
      capabilities?.querySelector('[data-home-workflows]')
    ).not.toBeInTheDocument()
  })

  test('uses six equally aligned capability rectangles', () => {
    const { container } = render(<Hero isAuthenticated={false} />)
    const layout = container.querySelector('[data-home-hero-layout]')
    const heroContent = container.querySelector('[data-home-hero-content]')
    const intro = container.querySelector('[data-home-hero-intro]')
    const heroCopy = container.querySelector('[data-home-hero-copy]')
    const tagline = heroCopy?.querySelector('h2')
    const valueStatement = container.querySelector(
      '[data-home-value-statement]'
    )
    const companyFooter = container.querySelector('[data-home-company-footer]')
    const capabilityGrid = container.querySelector(
      '[data-home-capability-grid]'
    )
    const capabilityTiles = container.querySelectorAll('[data-home-capability]')

    expect(layout).toHaveClass('flex', 'min-h-svh', 'flex-col')
    expect(heroContent).toHaveClass(
      'flex',
      'flex-1',
      'flex-col',
      'items-center',
      'justify-center'
    )
    expect(layout).not.toHaveClass(
      'lg:grid-cols-[minmax(0,0.88fr)_minmax(0,1.12fr)]'
    )
    expect(intro).toHaveClass('w-full', 'items-center', 'text-center')
    expect(tagline).toHaveClass('w-full')
    expect(tagline).toHaveClass('text-2xl', 'sm:text-3xl')
    expect(tagline).not.toHaveClass('whitespace-nowrap')
    expect(valueStatement).toHaveClass(
      'flex',
      'flex-wrap',
      'text-lg',
      'sm:text-xl'
    )
    expect(valueStatement).toHaveTextContent(
      'Bring more focus back to products'
    )
    expect(valueStatement).toHaveTextContent('Put more energy into research')
    expect(
      valueStatement?.querySelector('[data-home-value-focus="product"]')
    ).toHaveClass('text-emerald-700', 'dark:text-emerald-300')
    expect(
      valueStatement?.querySelector('[data-home-value-focus="research"]')
    ).toHaveClass('text-amber-700', 'dark:text-amber-300')
    expect(
      valueStatement?.querySelector('[data-home-value-phrase="product"]')
    ).toHaveClass('w-full', 'sm:w-auto')
    expect(
      valueStatement?.querySelector('[data-home-value-phrase="research"]')
    ).toHaveClass('w-full', 'sm:w-auto')
    expect(
      valueStatement?.querySelector('[data-home-value-separator]')
    ).toHaveClass('hidden', 'sm:inline')
    expect(capabilityGrid).toHaveClass(
      'auto-rows-fr',
      'grid-cols-1',
      'sm:grid-cols-2',
      'lg:grid-cols-3'
    )
    expect(capabilityGrid).not.toHaveClass('sm:grid-rows-2')
    expect(capabilityTiles).toHaveLength(6)
    expect(container.querySelectorAll('[data-home-glass-tile]')).toHaveLength(6)
    expect(container.querySelector('[data-home-use-case-list]')).toHaveClass(
      'text-sm',
      'sm:text-[15px]'
    )
    expect(companyFooter).toHaveClass(
      'mx-auto',
      'w-full',
      'max-w-5xl',
      'border-t',
      'text-center'
    )
    expect(companyFooter).toHaveTextContent(
      `© ${new Date().getFullYear()} 宙斯智算（上海）科技有限公司 All rights reserved`
    )
    expect(
      container.querySelector('[data-home-base-url]')
    ).not.toBeInTheDocument()
    capabilityTiles.forEach((tile) => {
      expect(tile).toHaveClass(
        'min-h-28',
        'min-w-0',
        'sm:h-full',
        'rounded-lg',
        'backdrop-blur-lg',
        'motion-safe:hover:-translate-y-1',
        'motion-reduce:transform-none',
        'motion-reduce:transition-none'
      )
      expect(tile.querySelector('h3')).toHaveClass(
        'text-base',
        'sm:text-lg',
        'break-words'
      )
      expect(tile.querySelector('h3')).not.toHaveClass(
        'truncate',
        'whitespace-nowrap'
      )
      expect(tile.querySelector('p')).toHaveClass(
        'text-sm',
        'sm:text-[15px]',
        'leading-6',
        'break-words'
      )
      expect(tile.querySelector('p')).not.toHaveClass(
        'truncate',
        'whitespace-nowrap'
      )
    })
    expect(companyFooter?.querySelector('p')).toHaveClass(
      'text-muted-foreground',
      'text-[13px]',
      'leading-5',
      'sm:text-sm'
    )
  })

  test('keeps the supporting copy free of terminal punctuation in English and Chinese', async () => {
    const { container } = render(<Hero isAuthenticated={false} />)
    const description = container.querySelector('[data-home-hero-description]')

    expect(description?.textContent).not.toMatch(/[.。]$/)

    await act(async () => {
      await i18next.changeLanguage('zhCN')
    })

    expect(description?.textContent).not.toMatch(/[.。]$/)
  })

  test('uses the current year in the localized company copyright', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2042-06-15T12:00:00Z'))

    const { container } = render(<Hero isAuthenticated={false} />)
    const companyFooter = container.querySelector('[data-home-company-footer]')

    expect(companyFooter).toHaveTextContent(
      '© 2042 宙斯智算（上海）科技有限公司 All rights reserved'
    )

    await act(async () => {
      await i18next.changeLanguage('zhCN')
    })

    expect(companyFooter).toHaveTextContent(
      '© 2042 宙斯智算（上海）科技有限公司 保留所有权利'
    )
  })

  test('updates the page heading after switching to Simplified Chinese', async () => {
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
    const heroCopy = document.querySelector('[data-home-hero-copy]')
    expect(heroCopy).toHaveTextContent(
      '面向企业开发者与科研团队的统一 AI 模型平台'
    )
    expect(heroCopy).toHaveTextContent('让更多思考回归产品')
    expect(heroCopy).toHaveTextContent('把更多精力投入研究')
    expect(capabilities).not.toHaveTextContent('面向团队的 AI 基础设施')
    expect(capabilities).toHaveTextContent('统一 API')
    expect(capabilities).toHaveTextContent('智能路由')
    expect(capabilities).toHaveTextContent('用量洞察')
    expect(capabilities).not.toHaveTextContent('API 地址')
    expect(capabilities).toHaveTextContent('按额度计费，支持对公开票')
    expect(
      [...(capabilities?.querySelectorAll('[data-home-use-case]') ?? [])].map(
        (item) => item.textContent?.trim()
      )
    ).toEqual(['智能代理', '代码开发', '内容写作', '资料整理', '数据分析'])
    expect(capabilities).not.toHaveTextContent('适用于')
    expect(screen.getByRole('list', { name: '应用场景' })).toBeVisible()
    expect(capabilities).not.toHaveTextContent('企业团队')
    expect(capabilities).not.toHaveTextContent('课题组')
    expect(capabilities).not.toHaveTextContent('支持对公发票')
    expect(capabilities).not.toHaveTextContent('支持人民币')
    expect(
      document.querySelector('[data-home-company-footer]')
    ).toHaveTextContent(
      `© ${new Date().getFullYear()} 宙斯智算（上海）科技有限公司 保留所有权利`
    )
    expect(document.querySelector('[data-home-hero]')).not.toHaveTextContent(
      '支持一键配置并完美适配 NewAPI 多协议配置'
    )
  })

  test('uses the original-logo brand lockup in the loading shell', () => {
    const { container } = render(<HeroLoadingShell />)
    const loadingHero = container.querySelector('[data-home-hero-loading]')

    expect(loadingHero).toContainElement(
      container.querySelector('[data-hero-brand-lockup]')
    )
    expect(loadingHero).toHaveClass('min-h-svh')
    expect(loadingHero).not.toHaveClass('h-svh')
    expect(loadingHero).not.toHaveClass('h-[calc(100svh-4rem)]')
    expect(
      screen.getByRole('heading', { level: 1, name: 'ZEUSZS' })
    ).toBeVisible()
  })

  test.each(['light', 'dark'] as const)(
    'keeps a theme-compatible star background without Earth or trajectory media in $appearance mode',
    (appearance) => {
      themeState.resolvedTheme = appearance
      const { container } = render(<Hero isAuthenticated={false} />)
      const hero = container.querySelector('[data-home-hero]')
      const background = container.querySelector('[data-home-space-background]')

      expect(hero).toHaveAttribute('data-hero-appearance', appearance)
      expect(background).toHaveAttribute('aria-hidden', 'true')
      expect(hero?.querySelector('canvas')).not.toBeInTheDocument()
      expect(hero?.querySelector('picture')).not.toBeInTheDocument()
      expect(
        hero?.querySelector('[data-orbital-earth-scene]')
      ).not.toBeInTheDocument()
      expect(hero?.querySelector('[data-earth-scene]')).not.toBeInTheDocument()
      expect(
        hero?.querySelector('[data-earth-poster-appearance]')
      ).not.toBeInTheDocument()
    }
  )

  test('keeps the loading shell free of retired scene media', () => {
    const { container } = render(<HeroLoadingShell />)
    const loadingHero = container.querySelector('[data-home-hero-loading]')

    expect(
      loadingHero?.querySelector('[data-home-space-background]')
    ).toHaveAttribute('aria-hidden', 'true')
    expect(loadingHero?.querySelector('canvas')).not.toBeInTheDocument()
    expect(loadingHero?.querySelector('picture')).not.toBeInTheDocument()
  })

  test('orders docs, primary, and pricing actions around the centered primary action', () => {
    const { container } = render(<Hero isAuthenticated={false} />)
    const actions = container.querySelector('[data-home-actions]')
    const docsAction = screen.getByRole('button', {
      name: 'Integration Docs',
    })
    const primaryAction = screen.getByRole('button', { name: 'Get Started' })
    const pricingAction = screen.getByRole('button', { name: 'Model Pricing' })

    expect(docsAction).toHaveAttribute('href', 'https://docs.zeuszs.example')
    expect(docsAction).toHaveAttribute('target', '_blank')
    expect(primaryAction).toHaveAttribute('href', '/sign-up')
    expect(pricingAction).toHaveAttribute('href', '/pricing')
    expect(
      [...(actions?.querySelectorAll('[data-home-action]') ?? [])].map((item) =>
        item.getAttribute('data-home-action')
      )
    ).toEqual(['docs', 'primary', 'pricing'])
    expect(actions).toHaveClass(
      'grid',
      'grid-cols-1',
      'sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]'
    )
    expect(docsAction).toHaveClass('sm:justify-self-end')
    expect(pricingAction).toHaveClass('sm:justify-self-start')
    expect(screen.queryByText('/v1/chat/completions')).not.toBeInTheDocument()
    expect(screen.queryByText(/200 ok/i)).not.toBeInTheDocument()
    expect(container.querySelector('[data-home-hero]')).not.toHaveTextContent(
      /storm|lightning/i
    )
  })

  test('keeps the dashboard as the centered primary action for signed-in users', () => {
    const { container } = render(<Hero isAuthenticated />)

    expect(
      screen.getByRole('button', { name: 'Go to Dashboard' })
    ).toHaveAttribute('href', '/dashboard')
    expect(
      [...container.querySelectorAll('[data-home-action]')].map((item) =>
        item.getAttribute('data-home-action')
      )
    ).toEqual(['docs', 'primary', 'pricing'])
  })
})
