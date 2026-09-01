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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { Home } from '../index'

const { mockUseHomePageContent, themeState } = vi.hoisted(() => ({
  mockUseHomePageContent: vi.fn(),
  themeState: { resolvedTheme: 'light' as 'dark' | 'light' },
}))

vi.mock('../hooks', () => ({
  useHomePageContent: mockUseHomePageContent,
}))

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: {
    children: ReactNode
    showMainContainer?: boolean
    headerProps?: {
      immersive?: boolean
      immersiveTone?: 'dark' | 'light'
      logo?: ReactNode
      siteName?: string
    }
  }) => (
    <div
      data-testid='public-layout'
      data-main-container={props.showMainContainer !== false}
      data-header-immersive={props.headerProps?.immersive}
      data-header-immersive-tone={props.headerProps?.immersiveTone}
      data-header-custom-logo={props.headerProps?.logo ? 'true' : 'false'}
      data-header-custom-name={props.headerProps?.siteName ? 'true' : 'false'}
    >
      {props.children}
    </div>
  ),
}))

vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer>footer</footer>,
}))

vi.mock('@/components/rich-content', () => ({
  RichContent: (props: {
    content: string
    mode?: 'html' | 'markdown'
    htmlVariant?: string
  }) => (
    <article
      data-testid='rich-content'
      data-mode={props.mode ?? 'markdown'}
      data-html-variant={props.htmlVariant}
    >
      {props.content}
    </article>
  ),
}))

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => themeState,
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: null } }),
}))

vi.mock('../components', () => ({
  Hero: () => <section data-home-hero>hero</section>,
  Stats: () => <section>stats</section>,
  Features: () => <section>features</section>,
  HowItWorks: () => <section>how it works</section>,
  CTA: () => <section>cta</section>,
}))

beforeEach(() => {
  themeState.resolvedTheme = 'light'
  mockUseHomePageContent.mockReturnValue({
    content: '',
    isLoaded: true,
    isUrl: false,
  })
})

describe('default home content routing', () => {
  test('shows a theme-matched loading shell beneath an immersive header while custom content is unresolved', () => {
    mockUseHomePageContent.mockReturnValue({
      content: '',
      isLoaded: false,
      isUrl: false,
    })

    render(<Home />)

    const loadingHero = document.querySelector('[data-home-hero-loading]')
    const layout = screen.getByTestId('public-layout')
    expect(loadingHero).toHaveAttribute('aria-busy', 'true')
    expect(loadingHero).toHaveAttribute('data-hero-appearance', 'light')
    expect(layout).toHaveAttribute('data-header-immersive', 'true')
    expect(layout).toHaveAttribute('data-header-immersive-tone', 'light')
    expect(layout).toHaveAttribute('data-header-custom-logo', 'false')
    expect(layout).toHaveAttribute('data-header-custom-name', 'false')
    expect(screen.getByRole('heading', { name: 'ZEUSZS' })).toBeVisible()
    expect(
      loadingHero?.querySelector('[data-home-space-background]')
    ).toHaveAttribute('aria-hidden', 'true')
    expect(document.querySelector('[data-home-hero]')).not.toBeInTheDocument()
  })

  test('renders only the full-screen hero when custom content is empty', () => {
    themeState.resolvedTheme = 'dark'

    render(<Home />)

    expect(document.querySelector('[data-home-hero]')).toBeInTheDocument()
    expect(screen.queryByText('stats')).not.toBeInTheDocument()
    expect(screen.queryByText('features')).not.toBeInTheDocument()
    expect(screen.queryByText('how it works')).not.toBeInTheDocument()
    expect(screen.queryByText('cta')).not.toBeInTheDocument()
    expect(screen.queryByText('footer')).not.toBeInTheDocument()
    expect(screen.queryByTestId('rich-content')).not.toBeInTheDocument()
    expect(screen.getByTestId('public-layout')).toHaveAttribute(
      'data-header-immersive-tone',
      'dark'
    )
    expect(screen.getByTestId('public-layout')).toHaveAttribute(
      'data-header-custom-logo',
      'false'
    )
    expect(screen.getByTestId('public-layout')).toHaveAttribute(
      'data-header-custom-name',
      'false'
    )
  })

  test('keeps non-empty Markdown in the configured content branch', () => {
    mockUseHomePageContent.mockReturnValue({
      content: '# Configured home',
      isLoaded: true,
      isUrl: false,
    })

    render(<Home />)

    expect(document.querySelector('[data-home-hero]')).not.toBeInTheDocument()
    expect(screen.getByTestId('rich-content')).toHaveAttribute(
      'data-mode',
      'markdown'
    )
    expect(screen.getByText('# Configured home')).toBeVisible()
    expect(screen.getByTestId('public-layout')).toHaveAttribute(
      'data-main-container',
      'true'
    )
  })

  test('keeps non-empty HTML in the isolated custom content branch', () => {
    mockUseHomePageContent.mockReturnValue({
      content: '<section>Configured home</section>',
      isLoaded: true,
      isUrl: false,
    })

    render(<Home />)

    expect(document.querySelector('[data-home-hero]')).not.toBeInTheDocument()
    expect(screen.getByTestId('rich-content')).toHaveAttribute(
      'data-mode',
      'html'
    )
    expect(screen.getByTestId('rich-content')).toHaveAttribute(
      'data-html-variant',
      'isolated'
    )
    expect(screen.getByTestId('public-layout')).toHaveAttribute(
      'data-main-container',
      'false'
    )
  })

  test('keeps a configured URL in the sandboxed iframe branch', () => {
    mockUseHomePageContent.mockReturnValue({
      content: 'https://example.com/custom-home',
      isLoaded: true,
      isUrl: true,
    })

    render(<Home />)

    expect(document.querySelector('[data-home-hero]')).not.toBeInTheDocument()
    expect(screen.queryByTestId('rich-content')).not.toBeInTheDocument()
    expect(screen.getByTitle('Custom Home Page')).toHaveAttribute(
      'src',
      'https://example.com/custom-home'
    )
    expect(screen.getByTitle('Custom Home Page')).toHaveAttribute(
      'sandbox',
      expect.stringContaining('allow-top-navigation-by-user-activation')
    )
  })
})
