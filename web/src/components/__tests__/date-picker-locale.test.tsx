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
import type { ComponentProps } from 'react'
import { enUS, zhCN } from 'react-day-picker/locale'
import { afterEach, describe, expect, test, vi } from 'vitest'

import type { Calendar } from '@/components/ui/calendar'

import { DatePicker } from '../date-picker'
import { DateTimePicker } from '../datetime-picker'

const mocks = vi.hoisted(() => ({
  calendarProps: [] as ComponentProps<typeof Calendar>[],
}))

vi.mock('@/components/ui/calendar', () => ({
  Calendar: (props: ComponentProps<typeof Calendar>) => {
    mocks.calendarProps.push(props)
    return <div data-testid='calendar' />
  },
}))

afterEach(async () => {
  mocks.calendarProps = []
  await i18next.changeLanguage('en')
})

async function expectCalendarLocale(
  language: string,
  expectedLocale: typeof enUS
) {
  await i18next.changeLanguage(language)

  render(
    <>
      <DatePicker
        selected={undefined}
        onSelect={() => {}}
        placeholder='Pick date'
      />
      <DateTimePicker placeholder='Pick date and time' />
    </>
  )

  fireEvent.click(screen.getByRole('button', { name: 'Pick date' }))
  fireEvent.click(screen.getByRole('button', { name: 'Pick date and time' }))

  await screen.findAllByTestId('calendar')
  expect(mocks.calendarProps.slice(-2).map((props) => props.locale)).toEqual([
    expectedLocale,
    expectedLocale,
  ])
}

describe('date picker locale', () => {
  test('uses Simplified Chinese for both date pickers', async () => {
    await expectCalendarLocale('zhCN', zhCN)
  })

  test.each(['en', 'fr'])(
    'uses English for both date pickers when the language is %s',
    async (language) => {
      await expectCalendarLocale(language, enUS)
    }
  )
})
