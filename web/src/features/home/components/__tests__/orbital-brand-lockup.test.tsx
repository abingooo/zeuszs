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
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import zhCN from '@/i18n/locales/zh.json'

import { OrbitalBrandLockup } from '../orbital-brand-lockup'

describe('ZEUSZS brand lockup', () => {
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
  })

  afterEach(async () => {
    await i18next.changeLanguage('en')
    i18next.removeResourceBundle('en', 'translation')
    i18next.removeResourceBundle('zhCN', 'translation')
  })

  test('shows the original ZEUSZS logo with a clean English text wordmark', () => {
    const { container } = render(<OrbitalBrandLockup />)

    const heading = screen.getByRole('heading', {
      level: 1,
      name: 'ZEUSZS',
    })
    expect(heading).toHaveAttribute('data-brand-language', 'en')
    expect(container.querySelector('[data-zeuszs-logo]')).toHaveAttribute(
      'src',
      '/zeuszs-logo.png'
    )
    expect(container.querySelector('[data-zeuszs-logo]')).toHaveClass(
      'object-contain'
    )
    const wordmark = container.querySelector('[data-brand-wordmark="en"]')
    expect(wordmark).toBeVisible()
    expect(wordmark).toHaveTextContent('ZEUSZS')
    expect(wordmark).toHaveClass('tracking-normal')
    expect(
      container.querySelector('[data-brand-wordmark="zhCN"]')
    ).not.toBeInTheDocument()
  })

  test('switches to a clean Chinese text wordmark when the interface language changes', async () => {
    const { container } = render(<OrbitalBrandLockup />)

    await act(async () => {
      await i18next.changeLanguage('zhCN')
    })

    const heading = screen.getByRole('heading', {
      level: 1,
      name: '宙斯智算',
    })
    expect(heading).toHaveAttribute('data-brand-language', 'zhCN')
    const wordmark = container.querySelector('[data-brand-wordmark="zhCN"]')
    expect(wordmark).toBeVisible()
    expect(wordmark).toHaveTextContent('宙斯智算')
    expect(wordmark).toHaveClass('tracking-normal')
    expect(
      container.querySelector('[data-brand-wordmark="en"]')
    ).not.toBeInTheDocument()
  })

  test('uses normal text without custom glyph geometry or effects', () => {
    const { container } = render(<OrbitalBrandLockup />)

    expect(container.querySelector('svg')).not.toBeInTheDocument()
    expect(container.querySelector('path')).not.toBeInTheDocument()
    expect(container.querySelector('[data-brand-wordmark]')).toHaveClass(
      'font-inter',
      'font-semibold',
      'tracking-normal'
    )
  })

  test('does not render the synthesized orbital Z mark', () => {
    const { container } = render(<OrbitalBrandLockup />)

    expect(container.querySelector('[data-brand-part="z"]')).toBeNull()
    expect(container.querySelector('[data-brand-part="orbit"]')).toBeNull()
    expect(container.querySelector('[data-brand-part="node"]')).toBeNull()
    expect(container.querySelector('[data-orbit-motion]')).toBeNull()
  })
})
