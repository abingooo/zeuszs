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

import {
  getOrganizationDisplayName,
  getOrganizationPinyinInitials,
} from '../organization-name'

describe('organization brand name', () => {
  test('converts a Chinese organization name to lowercase pinyin initials', () => {
    expect(getOrganizationPinyinInitials('宙斯智算')).toBe('zszs')
    expect(getOrganizationPinyinInitials('雷霆课题组')).toBe('ltktz')
  })

  test('uses the first letter of each Chinese syllable, including zh and sh', () => {
    expect(getOrganizationPinyinInitials('重庆重工实验室')).toBe('cqzgsys')
  })

  test('keeps mixed names identifiable while normalizing existing latin text', () => {
    expect(getOrganizationPinyinInitials('北京大学 AI 实验室')).toBe(
      'bjdxaisys'
    )
  })

  test('uses a lowercase readable fallback for names without Chinese characters', () => {
    expect(getOrganizationPinyinInitials('Research Team')).toBe('research team')
  })

  test('keeps the original name in Chinese and uses initials in English', () => {
    expect(getOrganizationDisplayName('宙斯智算', 'zhCN')).toBe('宙斯智算')
    expect(getOrganizationDisplayName('宙斯智算', 'zhTW')).toBe('宙斯智算')
    expect(getOrganizationDisplayName('宙斯智算', 'en')).toBe('zszs')
  })
})
