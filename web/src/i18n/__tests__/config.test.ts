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
import i18next, { type i18n as I18nInstance } from 'i18next'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { resources } from '../config'
import { INTERFACE_LANGUAGE_OPTIONS } from '../languages'

let i18n: I18nInstance

describe('i18n configuration', () => {
  beforeEach(async () => {
    i18n = i18next.createInstance()
    await i18n.init({
      resources,
      fallbackLng: 'en',
      supportedLngs: INTERFACE_LANGUAGE_OPTIONS.map(
        (language) => language.code
      ),
      load: 'currentOnly',
    })
  })

  afterEach(async () => {
    await i18n?.changeLanguage('en')
  })

  test('registers only the supported interface resources', () => {
    expect(Object.keys(resources).sort()).toEqual(['en', 'zhCN'])
    expect(
      [...((i18n.options.supportedLngs ?? []) as string[])]
        .filter((language) => language !== 'cimode')
        .sort()
    ).toEqual(['en', 'zhCN'])
  })

  test.each(['fr', 'ru', 'ja', 'vi', 'zhTW', 'zh-TW'])(
    'falls back to English for unsupported locale %s',
    async (locale) => {
      await i18n.changeLanguage(locale)

      expect(i18n.language).toBe('en')
      expect(i18n.resolvedLanguage).toBe('en')
    }
  )
})
