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

import { buildDashboardFlowData } from '../flow'

const rowWithoutUsername = {
  user_id: 42,
  use_group: 'default',
  model_name: 'gpt-test',
  channel_id: 3,
  quota: 100,
  token_used: 20,
  count: 1,
}

describe('dashboard flow user ID visibility', () => {
  test('does not use a numeric user ID as a visible label when IDs are hidden', () => {
    const result = buildDashboardFlowData([rowWithoutUsername], 'quota', {
      role: 'root',
      showInternalIds: false,
    })

    expect(result.filterOptions.users[0]?.label).toBe('Unknown User')
    expect(result.flow.nodes.find((node) => node.kind === 'user')?.label).toBe(
      'Unknown User'
    )
  })

  test('retains the numeric fallback when ID visibility is enabled', () => {
    const result = buildDashboardFlowData([rowWithoutUsername], 'quota', {
      role: 'root',
      showInternalIds: true,
    })

    expect(result.filterOptions.users[0]?.label).toBe('user-42')
    expect(result.flow.nodes.find((node) => node.kind === 'user')?.label).toBe(
      'user-42'
    )
  })
})
