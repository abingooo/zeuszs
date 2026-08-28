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
export type HeroSceneMode = 'interactive' | 'static'

export type HeroSceneAppearance = 'dark' | 'light'

export type HeroScenePhase = 'reveal' | 'approach' | 'orbit' | 'network'

export interface HeroSceneCapabilities {
  reducedMotion: boolean
  webGLAvailable: boolean
  saveData: boolean
  viewportWidth: number
  deviceMemory?: number
  hardwareConcurrency?: number
}

const MIN_INTERACTIVE_VIEWPORT_WIDTH = 768
const MIN_INTERACTIVE_DEVICE_MEMORY = 4
const MIN_INTERACTIVE_HARDWARE_CONCURRENCY = 4

export function getHeroSceneMode(
  capabilities: HeroSceneCapabilities
): HeroSceneMode {
  if (
    capabilities.reducedMotion ||
    !capabilities.webGLAvailable ||
    capabilities.saveData ||
    capabilities.viewportWidth < MIN_INTERACTIVE_VIEWPORT_WIDTH
  ) {
    return 'static'
  }

  if (
    capabilities.deviceMemory !== undefined &&
    capabilities.deviceMemory < MIN_INTERACTIVE_DEVICE_MEMORY
  ) {
    return 'static'
  }

  if (
    capabilities.hardwareConcurrency !== undefined &&
    capabilities.hardwareConcurrency < MIN_INTERACTIVE_HARDWARE_CONCURRENCY
  ) {
    return 'static'
  }

  return 'interactive'
}

export function clampHeroScrollProgress(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(Math.max(value, 0), 1)
}

export function getHeroScenePhase(progress: number): HeroScenePhase {
  const clampedProgress = clampHeroScrollProgress(progress)

  if (clampedProgress < 0.2) return 'reveal'
  if (clampedProgress < 0.48) return 'approach'
  if (clampedProgress < 0.76) return 'orbit'
  return 'network'
}
