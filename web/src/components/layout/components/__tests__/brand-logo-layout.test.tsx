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
import i18next from 'i18next'
import type React from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import zhCN from '@/i18n/locales/zh.json'

import { HeaderLogo } from '../header-logo'
import { SystemBrand } from '../system-brand'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: React.ComponentProps<'a'>) => <a {...props} />,
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { system_name: 'Deployment' } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    logo: '/wide-logo.png',
    systemName: 'Deployment',
  }),
}))

afterEach(async () => {
  await i18next.changeLanguage('en')
})

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

describe('brand logo layout', () => {
  test('keeps the deployment logo visible and the main brand independently prominent', () => {
    render(<SystemBrand variant='inline' />)

    expect(screen.getByRole('img', { name: 'Logo' })).toHaveClass(
      'object-contain'
    )
    expect(screen.getByText('Deployment')).toHaveAttribute(
      'data-slot',
      'system-brand-name'
    )
    expect(screen.getByText('Deployment')).toHaveClass(
      'shrink-0',
      'text-lg',
      'font-semibold'
    )
  })

  test('shows a truncating organization suffix after the main brand for a non-default organization', () => {
    render(
      <SystemBrand
        variant='inline'
        organization={{
          name: '雷霆课题组',
          is_default: false,
        }}
      />
    )

    const organization = screen.getByText('for ltktz')
    expect(organization).toHaveAttribute(
      'data-slot',
      'system-brand-organization'
    )
    expect(organization).toHaveClass('min-w-0', 'truncate', 'text-xs')
    expect(organization).toHaveAttribute('title', 'for ltktz')
  })

  test('uses the readable organization name with the English for connector in Chinese', async () => {
    i18next.addResourceBundle('zhCN', 'translation', zhCN.translation)
    await i18next.changeLanguage('zhCN')

    render(
      <SystemBrand
        variant='inline'
        organization={{ name: '雷霆课题组', is_default: false }}
      />
    )

    expect(screen.getByText('for 雷霆课题组')).toBeVisible()
  })

  test.each([
    ['default', true],
    ['unclassified', undefined],
  ])(
    'hides the organization suffix for a %s organization',
    (_label, isDefault) => {
      render(
        <SystemBrand
          variant='inline'
          organization={{
            name: 'Default Organization',
            is_default: isDefault,
          }}
        />
      )

      expect(
        screen.queryByText('for Default Organization')
      ).not.toBeInTheDocument()
    }
  )

  test('keeps a wide deployment logo fully visible in the public header', () => {
    render(<HeaderLogo src='/wide-logo.png' loading={false} logoLoaded />)

    expect(screen.getByRole('img', { name: 'logo' })).toHaveClass(
      'object-contain'
    )
  })
})
