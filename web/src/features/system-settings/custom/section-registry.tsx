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
/* eslint-disable react-refresh/only-export-components */
import type { CustomSettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'
import { IDVisibilitySection } from './id-visibility-section'

const CUSTOM_SECTIONS = [
  {
    id: 'id-visibility',
    titleKey: 'ID Visibility',
    build: (settings: CustomSettings) => (
      <IDVisibilitySection
        defaultValue={settings['custom_setting.id_visibility_enabled']}
      />
    ),
  },
] as const

export type CustomSectionId = (typeof CUSTOM_SECTIONS)[number]['id']

const customRegistry = createSectionRegistry<CustomSectionId, CustomSettings>({
  sections: CUSTOM_SECTIONS,
  defaultSection: 'id-visibility',
  basePath: '/system-settings/custom',
  urlStyle: 'path',
})

export const CUSTOM_SECTION_IDS = customRegistry.sectionIds
export const CUSTOM_DEFAULT_SECTION = customRegistry.defaultSection
export const getCustomSectionNavItems = customRegistry.getSectionNavItems
export const getCustomSectionContent = customRegistry.getSectionContent
export const getCustomSectionMeta = customRegistry.getSectionMeta
