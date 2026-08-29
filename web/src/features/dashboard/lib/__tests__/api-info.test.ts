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
import { describe, expect, test } from 'vitest'

import { getLatencyColorClass } from '../api-info'

describe('API info latency colors', () => {
  test('uses green below one second', () => {
    expect(getLatencyColorClass(999)).toBe('text-green-600 dark:text-green-400')
  })

  test('uses orange from one second up to three seconds', () => {
    expect(getLatencyColorClass(1000)).toBe(
      'text-orange-600 dark:text-orange-400'
    )
    expect(getLatencyColorClass(2999)).toBe(
      'text-orange-600 dark:text-orange-400'
    )
  })

  test('uses red at three seconds and above', () => {
    expect(getLatencyColorClass(3000)).toBe('text-red-600 dark:text-red-400')
  })
})
