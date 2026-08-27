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

import { buildApiParams } from '../utils'

describe('usage log organization filter', () => {
  test('sends organization scope only for the administrator log view', () => {
    const config = {
      page: 1,
      pageSize: 20,
      searchParams: { organizationId: '17' },
      columnFilters: [],
    }

    expect(buildApiParams({ ...config, isAdmin: true })).toMatchObject({
      organization_id: 17,
    })
    expect(buildApiParams({ ...config, isAdmin: false })).not.toHaveProperty(
      'organization_id'
    )
  })
})
