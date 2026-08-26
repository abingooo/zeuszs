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
import { renderHook } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { useSidebarData } from '../use-sidebar-data'

describe('sidebar data', () => {
  test('does not expose playground or chat navigation', () => {
    const { result } = renderHook(() => useSidebarData())

    expect(result.current.navGroups.some((group) => group.id === 'chat')).toBe(
      false
    )
    expect(
      result.current.navGroups
        .flatMap((group) => group.items)
        .some((item) => {
          if ('type' in item && item.type === 'chat-presets') return true
          return 'url' in item && item.url === '/playground'
        })
    ).toBe(false)
  })
})
