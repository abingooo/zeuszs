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
  convertDetectedLanguage,
  INTERFACE_LANGUAGE_OPTIONS,
  normalizeInterfaceLanguage,
} from '../languages'

describe('interface language support', () => {
  test('exposes only English and Simplified Chinese choices', () => {
    expect(INTERFACE_LANGUAGE_OPTIONS).toEqual([
      { code: 'zhCN', label: '简体中文' },
      { code: 'en', label: 'English' },
    ])
  })

  test.each([
    ['zh', 'zhCN'],
    ['zh-CN', 'zhCN'],
    ['zh-Hans', 'zhCN'],
    ['zh-TW', 'zhCN'],
    ['zh-Hant', 'zhCN'],
    ['zhTW', 'zhCN'],
    ['zhCN', 'zhCN'],
    ['en', 'en'],
    ['en-US', 'en'],
    ['EN_us', 'en'],
    ['fr', 'en'],
    ['ja', 'en'],
    ['ru', 'en'],
    ['vi', 'en'],
    ['unknown', 'en'],
    [null, 'en'],
    ['', 'en'],
  ] as const)('normalizes %s to %s', (value, expected) => {
    expect(normalizeInterfaceLanguage(value)).toBe(expected)
  })

  test.each([
    ['zh-CN', 'zhCN'],
    ['zh-TW', 'zhCN'],
    ['zh-Hant-TW', 'zhCN'],
    ['en-US', 'en'],
    ['fr-FR', 'en'],
  ] as const)('maps detected %s to a supported code', (value, expected) => {
    expect(convertDetectedLanguage(value)).toBe(expected)
  })
})
