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
export type PlatformRelease = {
  id: number
  tag_name: string
  name: string
  html_url: string
  body: string
  published_at: string
  prerelease: boolean
}

export type PlatformUpdaterStatus = {
  status: string
  tag?: string
  step?: string
  error?: string
  started_at: string | null
  finished_at: string | null
  updated_at?: string
}

export type PlatformUpdateData = {
  repository: string
  current_version: string
  latest_release: PlatformRelease
  update_available: boolean
  updater_configured: boolean
  updater_reachable: boolean
  updater_status: PlatformUpdaterStatus | null
  triggered_at?: string
}

export type PlatformUpdateResponse = {
  success: boolean
  message?: string
  code?: string
  data?: PlatformUpdateData | null
}
