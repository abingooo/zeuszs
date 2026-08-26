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
import type React from 'react'
import { describe, expect, test, vi } from 'vitest'

import { HeaderLogo } from '../header-logo'
import { SystemBrand } from '../system-brand'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: React.ComponentProps<'a'>) => <a {...props} />,
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { system_name: 'Deployment' } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ logo: '/wide-logo.png' }),
}))

describe('brand logo layout', () => {
  test('keeps a wide deployment logo fully visible in the app brand', () => {
    render(<SystemBrand variant='inline' />)

    expect(screen.getByRole('img', { name: 'Logo' })).toHaveClass(
      'object-contain'
    )
  })

  test('keeps a wide deployment logo fully visible in the public header', () => {
    render(
      <HeaderLogo
        src='/wide-logo.png'
        loading={false}
        logoLoaded
      />
    )

    expect(screen.getByRole('img', { name: 'logo' })).toHaveClass(
      'object-contain'
    )
  })
})
