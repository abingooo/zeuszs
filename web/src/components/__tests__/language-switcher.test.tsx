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
import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { LanguageSwitcher } from '../language-switcher'

describe('LanguageSwitcher', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en')
  })

  afterEach(async () => {
    await i18next.changeLanguage('en')
  })

  test('shows only English and Simplified Chinese choices', async () => {
    render(<LanguageSwitcher />)

    fireEvent.click(screen.getByRole('button', { name: 'Change language' }))

    const menuItems = await screen.findAllByRole('menuitem')
    expect(menuItems).toHaveLength(2)
    expect(screen.getByRole('menuitem', { name: '简体中文' })).toBeVisible()
    expect(screen.getByRole('menuitem', { name: 'English' })).toBeVisible()
    expect(screen.queryByRole('menuitem', { name: 'Français' })).toBeNull()
  })
})
