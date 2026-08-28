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
import { pinyin } from 'pinyin-pro'

const HAN_CHARACTER = /\p{Script=Han}/u
const NON_HAN_NAME_CHARACTERS = /[^\p{L}\p{N}]+/gu

/**
 * Convert an organization name to the compact identifier shown in English
 * tenant branding. Han characters use their pinyin first letter; existing
 * letters and numbers in mixed names are retained in lowercase. Names with
 * no Han characters are returned in lowercase as a readable fallback.
 */
export function getOrganizationPinyinInitials(name: string): string {
  const trimmedName = name.trim()
  if (!trimmedName) return ''
  if (!HAN_CHARACTER.test(trimmedName)) return trimmedName.toLowerCase()

  const parts = pinyin(trimmedName, {
    type: 'all',
    toneType: 'none',
    traditional: true,
    nonZh: 'consecutive',
  })

  const initials = parts
    .map((part) => {
      if (part.isZh) return part.first
      return part.origin.replace(NON_HAN_NAME_CHARACTERS, '')
    })
    .join('')
    .toLowerCase()

  return initials || trimmedName.toLowerCase()
}

/**
 * Chinese interfaces keep the organization name readable. English uses the
 * compact pinyin form so the tenant suffix remains suitable for the header.
 */
export function getOrganizationDisplayName(
  name: string,
  language?: string
): string {
  const trimmedName = name.trim()
  if (language?.toLowerCase().startsWith('en')) {
    return getOrganizationPinyinInitials(trimmedName)
  }
  return trimmedName
}
