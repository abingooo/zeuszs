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
  clampHeroScrollProgress,
  getHeroSceneMode,
  getHeroScenePhase,
  type HeroSceneCapabilities,
  type HeroScenePhase,
} from '../hero-scene-policy'

const interactiveCapabilities: HeroSceneCapabilities = {
  reducedMotion: false,
  webGLAvailable: true,
  saveData: false,
  viewportWidth: 1440,
  deviceMemory: 8,
  hardwareConcurrency: 8,
}

describe('hero scene mode', () => {
  test('uses the interactive scene when every known capability meets the threshold', () => {
    expect(getHeroSceneMode(interactiveCapabilities)).toBe('interactive')
  })

  test('does not downgrade when optional hardware information is unknown', () => {
    expect(
      getHeroSceneMode({
        ...interactiveCapabilities,
        deviceMemory: undefined,
        hardwareConcurrency: undefined,
      })
    ).toBe('interactive')
  })

  test.each<{
    name: string
    capabilities: Partial<HeroSceneCapabilities>
  }>([
    {
      name: 'reduced motion is requested',
      capabilities: { reducedMotion: true },
    },
    {
      name: 'WebGL is unavailable',
      capabilities: { webGLAvailable: false },
    },
    {
      name: 'data saving is enabled',
      capabilities: { saveData: true },
    },
    {
      name: 'the viewport is narrower than 768 pixels',
      capabilities: { viewportWidth: 767 },
    },
    {
      name: 'device memory is below 4 GiB',
      capabilities: { deviceMemory: 3 },
    },
    {
      name: 'hardware concurrency is below four threads',
      capabilities: { hardwareConcurrency: 3 },
    },
  ])('uses the static scene when $name', ({ capabilities }) => {
    expect(
      getHeroSceneMode({ ...interactiveCapabilities, ...capabilities })
    ).toBe('static')
  })

  test('keeps exact viewport and hardware thresholds interactive', () => {
    expect(
      getHeroSceneMode({
        ...interactiveCapabilities,
        viewportWidth: 768,
        deviceMemory: 4,
        hardwareConcurrency: 4,
      })
    ).toBe('interactive')
  })
})

describe('hero scroll progress', () => {
  test.each([
    { value: -1, expected: 0 },
    { value: 0, expected: 0 },
    { value: 0.42, expected: 0.42 },
    { value: 1, expected: 1 },
    { value: 2, expected: 1 },
    { value: Number.NaN, expected: 0 },
    { value: Number.POSITIVE_INFINITY, expected: 0 },
    { value: Number.NEGATIVE_INFINITY, expected: 0 },
  ])('clamps $value to $expected', ({ value, expected }) => {
    expect(clampHeroScrollProgress(value)).toBe(expected)
  })
})

describe('hero scene phase', () => {
  test.each<{ progress: number; expected: HeroScenePhase }>([
    { progress: 0, expected: 'reveal' },
    { progress: 0.199, expected: 'reveal' },
    { progress: 0.2, expected: 'approach' },
    { progress: 0.479, expected: 'approach' },
    { progress: 0.48, expected: 'orbit' },
    { progress: 0.759, expected: 'orbit' },
    { progress: 0.76, expected: 'network' },
    { progress: 1, expected: 'network' },
  ])(
    'maps progress $progress to the $expected phase',
    ({ progress, expected }) => {
      expect(getHeroScenePhase(progress)).toBe(expected)
    }
  )

  test.each<{ progress: number; expected: HeroScenePhase }>([
    { progress: -1, expected: 'reveal' },
    { progress: 2, expected: 'network' },
    { progress: Number.NaN, expected: 'reveal' },
  ])(
    'normalizes out-of-range progress $progress before selecting $expected',
    ({ progress, expected }) => {
      expect(getHeroScenePhase(progress)).toBe(expected)
    }
  )
})
