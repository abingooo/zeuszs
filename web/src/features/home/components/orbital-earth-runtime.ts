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
import * as THREE from 'three'

import earthCloudsUrl from '@/assets/home/earth-clouds-4k.webp'
import earthDayUrl from '@/assets/home/earth-day-4k.webp'
import earthNightUrl from '@/assets/home/earth-night-4k.webp'

import type { HeroSceneAppearance } from '../lib/hero-scene-policy'

interface OrbitalEarthRuntimeOptions {
  appearanceRef: { current: HeroSceneAppearance }
  canvas: HTMLCanvasElement
  container: HTMLElement
  progressRef: { current: number }
  signal: AbortSignal
  onReady: () => void
  onContextLost: () => void
}

interface NetworkPath {
  curve: THREE.QuadraticBezierCurve3
  line: THREE.Line
  pulse: THREE.Mesh
  offset: number
  speed: number
}

interface ObservationNode {
  mesh: THREE.Mesh
  radiusX: number
  radiusY: number
  offset: number
  speed: number
}

interface ObservatoryField {
  group: THREE.Group
  lines: THREE.Line[]
  fixedNodes: THREE.Mesh[]
  movingNodes: ObservationNode[]
}

const EARTH_RADIUS = 5
const TARGET_FRAME_INTERVAL = 1000 / 60
const MAX_DRAWING_BUFFER_PIXELS = 6_000_000
const MAX_DEVICE_PIXEL_RATIO = 2

const EARTH_VERTEX_SHADER = `
  varying vec2 vUv;
  varying vec3 vWorldNormal;
  varying vec3 vWorldPosition;

  void main() {
    vUv = uv;
    vec4 worldPosition = modelMatrix * vec4(position, 1.0);
    vWorldPosition = worldPosition.xyz;
    vWorldNormal = normalize(mat3(modelMatrix) * normal);
    gl_Position = projectionMatrix * viewMatrix * worldPosition;
  }
`

const EARTH_FRAGMENT_SHADER = `
  uniform sampler2D dayMap;
  uniform sampler2D nightMap;
  uniform vec3 sunDirection;
  uniform float appearance;
  uniform float reveal;

  varying vec2 vUv;
  varying vec3 vWorldNormal;
  varying vec3 vWorldPosition;

  void main() {
    vec3 normal = normalize(vWorldNormal);
    vec3 viewDirection = normalize(cameraPosition - vWorldPosition);
    float sunDot = dot(normal, sunDirection);
    float sunlight = smoothstep(-0.18, 0.28, sunDot);
    float darkness = 1.0 - smoothstep(-0.2, 0.1, sunDot);
    float litRim = pow(1.0 - max(dot(normal, viewDirection), 0.0), 7.2)
      * smoothstep(-0.08, 0.46, sunDot);

    vec3 day = texture2D(dayMap, vUv).rgb;
    vec3 night = texture2D(nightMap, vUv).rgb;
    vec3 darkSurface = day * (0.07 + sunlight * 0.93)
      + night * darkness * 1.14;
    vec3 lightSurface = day * (0.26 + sunlight * 0.78)
      + night * darkness * 0.42;

    vec3 surface = mix(darkSurface, lightSurface, appearance);
    vec3 rimColor = mix(vec3(1.0, 0.86, 0.65), vec3(1.0, 0.98, 0.92), appearance);
    surface += rimColor * litRim * mix(0.12, 0.08, appearance);
    gl_FragColor = vec4(surface * reveal, 1.0);
  }
`

const CLOUD_FRAGMENT_SHADER = `
  uniform sampler2D cloudMap;
  uniform vec3 sunDirection;
  uniform float appearance;
  uniform float reveal;

  varying vec2 vUv;
  varying vec3 vWorldNormal;
  varying vec3 vWorldPosition;

  void main() {
    vec3 normal = normalize(vWorldNormal);
    vec3 viewDirection = normalize(cameraPosition - vWorldPosition);
    float sunlight = smoothstep(-0.36, 0.34, dot(normal, sunDirection));
    float cloud = smoothstep(0.24, 0.8, texture2D(cloudMap, vUv).r);
    float grazing = pow(1.0 - max(dot(normal, viewDirection), 0.0), 2.0);

    vec3 darkCloud = mix(vec3(0.16, 0.17, 0.18), vec3(0.9, 0.88, 0.84), sunlight);
    vec3 lightCloud = mix(vec3(0.48, 0.5, 0.51), vec3(0.98, 0.97, 0.94), sunlight);
    vec3 cloudColor = mix(darkCloud, lightCloud, appearance);
    float alpha = cloud * mix(0.34, 0.29, appearance)
      * mix(0.38, 1.0, max(sunlight, appearance * 0.62))
      * mix(1.0, 1.12, grazing);

    gl_FragColor = vec4(cloudColor, alpha * reveal);
  }
`

function createSeededRandom(seed: number): () => number {
  return () => {
    seed = (1664525 * seed + 1013904223) >>> 0
    return seed / 4294967296
  }
}

function createStarField(
  count: number,
  spread: number,
  depth: number,
  size: number,
  seed: number
): THREE.Points {
  const random = createSeededRandom(seed)
  const positions = new Float32Array(count * 3)

  for (let index = 0; index < count; index += 1) {
    const offset = index * 3
    positions[offset] = (random() - 0.5) * spread
    positions[offset + 1] = (random() - 0.35) * spread * 0.5
    positions[offset + 2] = -5 - random() * depth
  }

  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  const material = new THREE.PointsMaterial({
    color: 0xdcecff,
    size,
    sizeAttenuation: true,
    transparent: true,
    opacity: 0,
    depthWrite: false,
  })

  return new THREE.Points(geometry, material)
}

function createObservatoryField(): ObservatoryField {
  const group = new THREE.Group()
  const lines: THREE.Line[] = []
  const fixedNodes: THREE.Mesh[] = []
  const movingNodes: ObservationNode[] = []
  const orbitConfigurations = [
    {
      radiusX: 9.8,
      radiusY: 4.2,
      rotation: -0.18,
      start: -2.88,
      end: 2.5,
      darkOpacity: 0.105,
      lightOpacity: 0.09,
    },
    {
      radiusX: 12.4,
      radiusY: 5.7,
      rotation: 0.34,
      start: -2.38,
      end: 2.96,
      darkOpacity: 0.07,
      lightOpacity: 0.065,
    },
    {
      radiusX: 15.1,
      radiusY: 7.4,
      rotation: -0.48,
      start: -1.92,
      end: 2.42,
      darkOpacity: 0.045,
      lightOpacity: 0.05,
    },
  ]

  orbitConfigurations.forEach((config, orbitIndex) => {
    const orbit = new THREE.Group()
    orbit.rotation.z = config.rotation
    group.add(orbit)

    const points: THREE.Vector3[] = []
    for (let index = 0; index <= 160; index += 1) {
      const progress = index / 160
      const angle = THREE.MathUtils.lerp(config.start, config.end, progress)
      points.push(
        new THREE.Vector3(
          Math.cos(angle) * config.radiusX,
          Math.sin(angle) * config.radiusY,
          0
        )
      )
    }
    const lineMaterial = new THREE.LineBasicMaterial({
      color: 0xf1dec1,
      transparent: true,
      opacity: 0,
      depthWrite: false,
    })
    lineMaterial.userData.darkOpacity = config.darkOpacity
    lineMaterial.userData.lightOpacity = config.lightOpacity
    const line = new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(points),
      lineMaterial
    )
    lines.push(line)
    orbit.add(line)

    const tickPositions: number[] = []
    const tickCount = [19, 13, 9][orbitIndex] ?? 9
    for (let index = 1; index < tickCount; index += 1) {
      const progress = index / tickCount
      const angle = THREE.MathUtils.lerp(config.start, config.end, progress)
      const tickLength = index % 4 === 0 ? 0.18 : 0.09
      const x = Math.cos(angle) * config.radiusX
      const y = Math.sin(angle) * config.radiusY
      const normal = new THREE.Vector2(
        Math.cos(angle) / config.radiusX,
        Math.sin(angle) / config.radiusY
      ).normalize()
      tickPositions.push(
        x - normal.x * tickLength,
        y - normal.y * tickLength,
        0,
        x + normal.x * tickLength,
        y + normal.y * tickLength,
        0
      )
    }
    const tickGeometry = new THREE.BufferGeometry()
    tickGeometry.setAttribute(
      'position',
      new THREE.Float32BufferAttribute(tickPositions, 3)
    )
    const tickMaterial = new THREE.LineBasicMaterial({
      color: 0xf1dec1,
      transparent: true,
      opacity: 0,
      depthWrite: false,
    })
    tickMaterial.userData.darkOpacity = config.darkOpacity * 1.28
    tickMaterial.userData.lightOpacity = config.lightOpacity * 1.22
    const ticks = new THREE.LineSegments(tickGeometry, tickMaterial)
    lines.push(ticks)
    orbit.add(ticks)

    const fixedAngles = orbitIndex === 0 ? [-1.72, 0.38, 2.04] : [0.84]
    for (const angle of fixedAngles) {
      const nodeMaterial = new THREE.MeshBasicMaterial({
        color: 0xf0c986,
        transparent: true,
        opacity: 0,
        depthWrite: false,
      })
      nodeMaterial.userData.darkOpacity = orbitIndex === 0 ? 0.66 : 0.42
      nodeMaterial.userData.lightOpacity = orbitIndex === 0 ? 0.5 : 0.34
      const node = new THREE.Mesh(
        new THREE.SphereGeometry(orbitIndex === 0 ? 0.052 : 0.04, 10, 8),
        nodeMaterial
      )
      node.position.set(
        Math.cos(angle) * config.radiusX,
        Math.sin(angle) * config.radiusY,
        0
      )
      fixedNodes.push(node)
      orbit.add(node)
    }

    if (orbitIndex < 2) {
      const nodeMaterial = new THREE.MeshBasicMaterial({
        color: 0xffdfa5,
        transparent: true,
        opacity: 0,
        depthWrite: false,
      })
      nodeMaterial.userData.darkOpacity = orbitIndex === 0 ? 0.92 : 0.68
      nodeMaterial.userData.lightOpacity = orbitIndex === 0 ? 0.76 : 0.54
      const node = new THREE.Mesh(
        new THREE.SphereGeometry(orbitIndex === 0 ? 0.072 : 0.058, 12, 10),
        nodeMaterial
      )
      orbit.add(node)
      movingNodes.push({
        mesh: node,
        radiusX: config.radiusX,
        radiusY: config.radiusY,
        offset: orbitIndex === 0 ? 0.18 : 2.38,
        speed: (Math.PI * 2) / (orbitIndex === 0 ? 10.5 : 13.2),
      })
    }
  })

  group.position.set(1.25, -0.42, -6.8)
  return { group, lines, fixedNodes, movingNodes }
}

function pointOnVisibleHemisphere(x: number, y: number, lift: number) {
  const z = Math.sqrt(Math.max(EARTH_RADIUS ** 2 - x ** 2 - y ** 2, 0))
  return new THREE.Vector3(x, y, z + lift)
}

function createNetworkPaths(): {
  group: THREE.Group
  paths: NetworkPath[]
} {
  const group = new THREE.Group()
  const paths: NetworkPath[] = []
  const endpoints = [
    [
      pointOnVisibleHemisphere(-1.08, -0.7, 0.08),
      new THREE.Vector3(-3.1, -1.35, 4.25),
    ],
    [
      pointOnVisibleHemisphere(-0.36, -0.62, 0.08),
      new THREE.Vector3(-2.2, -2.55, 3.95),
    ],
    [
      pointOnVisibleHemisphere(0.48, -0.74, 0.08),
      new THREE.Vector3(2.6, -2.3, 4.14),
    ],
    [
      pointOnVisibleHemisphere(1.18, -0.32, 0.08),
      new THREE.Vector3(3.42, -0.74, 3.9),
    ],
  ] as const

  endpoints.forEach(([start, end], index) => {
    const control = start.clone().lerp(end, 0.5).multiplyScalar(1.14)
    const curve = new THREE.QuadraticBezierCurve3(start, control, end)
    const lineGeometry = new THREE.BufferGeometry().setFromPoints(
      curve.getPoints(96)
    )
    const lineMaterial = new THREE.LineBasicMaterial({
      color: 0xe8bd78,
      transparent: true,
      opacity: 0,
      blending: THREE.NormalBlending,
      depthWrite: false,
    })
    const line = new THREE.Line(lineGeometry, lineMaterial)

    const pulseMaterial = new THREE.MeshBasicMaterial({
      color: 0xffe2ad,
      transparent: true,
      opacity: 0,
      blending: THREE.NormalBlending,
      depthWrite: false,
    })
    const pulse = new THREE.Mesh(
      new THREE.SphereGeometry(0.036, 10, 8),
      pulseMaterial
    )
    pulse.visible = false

    group.add(line, pulse)
    paths.push({
      curve,
      line,
      pulse,
      offset: index * 0.23,
      speed: 0.09 + index * 0.012,
    })
  })

  return { group, paths }
}

function createOrbitGuides(): THREE.Group {
  const group = new THREE.Group()
  const configurations = [
    { radius: 5.3, rotationX: 0.52, rotationY: -0.18, opacity: 0.11 },
    { radius: 5.42, rotationX: -0.36, rotationY: 0.34, opacity: 0.07 },
  ]

  for (const config of configurations) {
    const points: THREE.Vector3[] = []
    for (let index = 0; index < 160; index += 1) {
      const angle = (index / 160) * Math.PI * 2
      points.push(
        new THREE.Vector3(
          Math.cos(angle) * config.radius,
          Math.sin(angle) * config.radius,
          0
        )
      )
    }
    const geometry = new THREE.BufferGeometry().setFromPoints(points)
    const material = new THREE.LineBasicMaterial({
      color: 0xe8bd78,
      transparent: true,
      opacity: 0,
      depthWrite: false,
    })
    material.userData.baseOpacity = config.opacity
    const orbit = new THREE.LineLoop(geometry, material)
    orbit.rotation.x = config.rotationX
    orbit.rotation.y = config.rotationY
    group.add(orbit)
  }

  return group
}

function smoothStep(edge0: number, edge1: number, value: number): number {
  const progress = THREE.MathUtils.clamp(
    (value - edge0) / (edge1 - edge0),
    0,
    1
  )
  return progress * progress * (3 - 2 * progress)
}

export async function startOrbitalEarthRuntime(
  options: OrbitalEarthRuntimeOptions
): Promise<() => void> {
  const textureLoader = new THREE.TextureLoader()
  const textureResults = await Promise.allSettled([
    textureLoader.loadAsync(earthDayUrl),
    textureLoader.loadAsync(earthNightUrl),
    textureLoader.loadAsync(earthCloudsUrl),
  ])
  const loadedTextures = textureResults.flatMap((result) =>
    result.status === 'fulfilled' ? [result.value] : []
  )
  const failedTexture = textureResults.find(
    (result): result is PromiseRejectedResult => result.status === 'rejected'
  )

  if (failedTexture || options.signal.aborted) {
    for (const texture of loadedTextures) texture.dispose()
    if (failedTexture) throw failedTexture.reason
    return () => undefined
  }

  const [dayMap, nightMap, cloudMap] = loadedTextures
  const initialWidth = Math.max(options.container.clientWidth, 1)
  const initialHeight = Math.max(options.container.clientHeight, 1)
  const initialPixelRatio = Math.max(
    0.25,
    Math.min(
      window.devicePixelRatio || 1,
      MAX_DEVICE_PIXEL_RATIO,
      Math.sqrt(MAX_DRAWING_BUFFER_PIXELS / (initialWidth * initialHeight))
    )
  )
  const initialDrawingBufferPixels =
    initialWidth * initialHeight * initialPixelRatio ** 2

  let renderer: THREE.WebGLRenderer
  try {
    renderer = new THREE.WebGLRenderer({
      canvas: options.canvas,
      alpha: false,
      antialias: initialDrawingBufferPixels <= MAX_DRAWING_BUFFER_PIXELS / 2,
      failIfMajorPerformanceCaveat: true,
      powerPreference: 'high-performance',
    })
  } catch (error) {
    for (const texture of loadedTextures) texture.dispose()
    throw error
  }
  renderer.outputColorSpace = THREE.SRGBColorSpace
  renderer.toneMapping = THREE.ACESFilmicToneMapping

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(38, 1, 0.1, 100)
  camera.position.set(0, 0.1, 13.5)
  camera.lookAt(0, -0.45, 0)

  dayMap.colorSpace = THREE.SRGBColorSpace
  nightMap.colorSpace = THREE.SRGBColorSpace
  cloudMap.colorSpace = THREE.NoColorSpace

  const maxAnisotropy = renderer.capabilities.getMaxAnisotropy()
  for (const texture of [dayMap, nightMap, cloudMap]) {
    texture.anisotropy = Math.min(maxAnisotropy, 12)
    texture.magFilter = THREE.LinearFilter
    texture.minFilter = THREE.LinearMipmapLinearFilter
    texture.wrapS = THREE.RepeatWrapping
  }

  const initialAppearance = options.appearanceRef.current === 'light' ? 1 : 0
  const sunDirection = new THREE.Vector3(0.86, 0.12, 0.3).normalize()
  const earthGroup = new THREE.Group()
  earthGroup.position.set(3.55, -4.45, 0)
  earthGroup.rotation.set(-0.68, 0, -0.16)
  scene.add(earthGroup)

  const earthGeometry = new THREE.SphereGeometry(EARTH_RADIUS, 128, 96)
  const earthMaterial = new THREE.ShaderMaterial({
    vertexShader: EARTH_VERTEX_SHADER,
    fragmentShader: EARTH_FRAGMENT_SHADER,
    uniforms: {
      appearance: { value: initialAppearance },
      dayMap: { value: dayMap },
      nightMap: { value: nightMap },
      reveal: { value: 0 },
      sunDirection: { value: sunDirection },
    },
  })
  const earthMesh = new THREE.Mesh(earthGeometry, earthMaterial)
  earthMesh.rotation.y = -0.62
  earthGroup.add(earthMesh)

  const cloudGeometry = new THREE.SphereGeometry(EARTH_RADIUS * 1.009, 128, 96)
  const cloudMaterial = new THREE.ShaderMaterial({
    vertexShader: EARTH_VERTEX_SHADER,
    fragmentShader: CLOUD_FRAGMENT_SHADER,
    transparent: true,
    depthWrite: false,
    uniforms: {
      appearance: { value: initialAppearance },
      cloudMap: { value: cloudMap },
      reveal: { value: 0 },
      sunDirection: { value: sunDirection },
    },
  })
  const cloudMesh = new THREE.Mesh(cloudGeometry, cloudMaterial)
  earthGroup.add(cloudMesh)

  const orbitGuides = createOrbitGuides()
  earthGroup.add(orbitGuides)
  const network = createNetworkPaths()
  earthGroup.add(network.group)

  const farStars = createStarField(170, 44, 28, 0.034, 0x5a455553)
  const nearStars = createStarField(64, 32, 16, 0.052, 0x4f524249)
  const observatory = createObservatoryField()
  scene.add(observatory.group, farStars, nearStars)
  const farStarMaterial = farStars.material as THREE.PointsMaterial
  const nearStarMaterial = nearStars.material as THREE.PointsMaterial

  const darkClear = new THREE.Color(0x121a27)
  const lightClear = new THREE.Color(0xedf3f7)
  const darkStar = new THREE.Color(0xf5eee3)
  const lightStar = new THREE.Color(0x607184)
  const darkTrace = new THREE.Color(0xe8bd78)
  const lightTrace = new THREE.Color(0x835b2f)
  const darkPulse = new THREE.Color(0xffe2ad)
  const lightPulse = new THREE.Color(0x6e461d)
  const darkObservatoryLine = new THREE.Color(0xf0dfc3)
  const lightObservatoryLine = new THREE.Color(0x4e6276)
  const darkObservatoryNode = new THREE.Color(0xf2c77e)
  const lightObservatoryNode = new THREE.Color(0x855a28)
  const clearColor = darkClear.clone().lerp(lightClear, initialAppearance)
  renderer.setClearColor(clearColor, 1)

  let animationFrame = 0
  let lastFrameTime = 0
  let previousRenderTime = 0
  let running = false
  let sceneVisible = true
  let disposed = false
  let readyReported = false
  let appearanceMix = initialAppearance
  const startedAt = performance.now()
  const pointerTarget = new THREE.Vector2()
  const pointerCurrent = new THREE.Vector2()
  const pointerClient = new THREE.Vector2(
    Number.NEGATIVE_INFINITY,
    Number.NEGATIVE_INFINITY
  )
  let containerBounds: DOMRect | undefined
  let boundsDirty = true
  let pointerDirty = false

  const resize = () => {
    const width = Math.max(options.container.clientWidth, 1)
    const height = Math.max(options.container.clientHeight, 1)
    const pixelBudgetRatio = Math.sqrt(
      MAX_DRAWING_BUFFER_PIXELS / (width * height)
    )
    const pixelRatio = Math.max(
      0.25,
      Math.min(
        window.devicePixelRatio || 1,
        MAX_DEVICE_PIXEL_RATIO,
        pixelBudgetRatio
      )
    )
    if (Math.abs(renderer.getPixelRatio() - pixelRatio) > 0.01) {
      renderer.setPixelRatio(pixelRatio)
    }
    renderer.setSize(width, height, false)
    camera.aspect = width / height
    camera.updateProjectionMatrix()
    boundsDirty = true
  }

  const onPointerMove = (event: PointerEvent) => {
    pointerClient.set(event.clientX, event.clientY)
    pointerDirty = true
  }

  const onViewportShift = () => {
    boundsDirty = true
  }

  const updatePointerTarget = () => {
    if (boundsDirty || !containerBounds) {
      containerBounds = options.container.getBoundingClientRect()
      boundsDirty = false
      pointerDirty = true
    }
    if (!pointerDirty) return
    pointerDirty = false

    const rect = containerBounds
    if (
      pointerClient.x < rect.left ||
      pointerClient.x > rect.right ||
      pointerClient.y < rect.top ||
      pointerClient.y > rect.bottom
    ) {
      pointerTarget.set(0, 0)
      return
    }
    pointerTarget.set(
      ((pointerClient.x - rect.left) / Math.max(rect.width, 1) - 0.5) * 2,
      ((pointerClient.y - rect.top) / Math.max(rect.height, 1) - 0.5) * 2
    )
  }

  const renderFrame = (now: number) => {
    if (!running || disposed) return
    animationFrame = window.requestAnimationFrame(renderFrame)
    const frameElapsed = now - lastFrameTime
    if (frameElapsed < TARGET_FRAME_INTERVAL - 1) return

    const deltaSeconds = Math.min(
      Math.max((now - (previousRenderTime || now)) / 1000, 0),
      0.1
    )
    previousRenderTime = now
    lastFrameTime = now - (frameElapsed % TARGET_FRAME_INTERVAL)
    updatePointerTarget()

    const elapsed = Math.max((now - startedAt) / 1000, 0)
    const progress = THREE.MathUtils.clamp(options.progressRef.current, 0, 1)
    const reveal = smoothStep(0, 1.25, elapsed)
    const approach = smoothStep(0.12, 0.52, progress)
    const orbit = smoothStep(0.34, 0.72, progress)
    const networkStrength = smoothStep(0.58, 0.94, progress)
    const appearanceTarget = options.appearanceRef.current === 'light' ? 1 : 0
    appearanceMix = THREE.MathUtils.lerp(
      appearanceMix,
      appearanceTarget,
      1 - Math.exp(-deltaSeconds * 5.5)
    )
    pointerCurrent.lerp(pointerTarget, 1 - Math.exp(-deltaSeconds * 4.6))

    earthMaterial.uniforms.appearance.value = appearanceMix
    earthMaterial.uniforms.reveal.value = reveal
    cloudMaterial.uniforms.appearance.value = appearanceMix
    cloudMaterial.uniforms.reveal.value = reveal

    clearColor.copy(darkClear).lerp(lightClear, appearanceMix)
    renderer.setClearColor(clearColor, 1)
    renderer.toneMappingExposure = THREE.MathUtils.lerp(
      1.16,
      1.04,
      appearanceMix
    )
    farStarMaterial.color.copy(darkStar).lerp(lightStar, appearanceMix)
    nearStarMaterial.color.copy(darkStar).lerp(lightStar, appearanceMix)
    farStarMaterial.opacity =
      reveal * THREE.MathUtils.lerp(0.46, 0.18, appearanceMix)
    nearStarMaterial.opacity =
      reveal * THREE.MathUtils.lerp(0.34, 0.12, appearanceMix)

    earthMesh.rotation.y = -0.62 + elapsed * 0.018 + progress * 0.16
    cloudMesh.rotation.y = -0.62 + elapsed * 0.025 + progress * 0.09
    farStars.rotation.y = elapsed * 0.0014
    nearStars.rotation.y = elapsed * -0.0022
    nearStars.position.x = Math.sin(elapsed * 0.18) * 0.08

    observatory.group.position.x = 1.25 - pointerCurrent.x * 0.16
    observatory.group.position.y = -0.42 + pointerCurrent.y * 0.1
    observatory.group.rotation.z = elapsed * 0.0008 + progress * 0.014
    for (const line of observatory.lines) {
      const material = line.material as THREE.LineBasicMaterial
      const opacity = THREE.MathUtils.lerp(
        material.userData.darkOpacity as number,
        material.userData.lightOpacity as number,
        appearanceMix
      )
      material.color
        .copy(darkObservatoryLine)
        .lerp(lightObservatoryLine, appearanceMix)
      material.opacity = reveal * opacity
    }
    for (const node of observatory.fixedNodes) {
      const material = node.material as THREE.MeshBasicMaterial
      const opacity = THREE.MathUtils.lerp(
        material.userData.darkOpacity as number,
        material.userData.lightOpacity as number,
        appearanceMix
      )
      material.color
        .copy(darkObservatoryNode)
        .lerp(lightObservatoryNode, appearanceMix)
      material.opacity = reveal * opacity
    }
    for (const node of observatory.movingNodes) {
      const angle = node.offset + elapsed * node.speed
      node.mesh.position.set(
        Math.cos(angle) * node.radiusX,
        Math.sin(angle) * node.radiusY,
        0
      )
      const material = node.mesh.material as THREE.MeshBasicMaterial
      const opacity = THREE.MathUtils.lerp(
        material.userData.darkOpacity as number,
        material.userData.lightOpacity as number,
        appearanceMix
      )
      material.color
        .copy(darkObservatoryNode)
        .lerp(lightObservatoryNode, appearanceMix)
      material.opacity = reveal * opacity
      node.mesh.scale.setScalar(0.92 + Math.sin(elapsed * 1.8) * 0.08)
    }

    camera.position.x = pointerCurrent.x * 0.2
    camera.position.z = THREE.MathUtils.lerp(13.5, 11.4, approach)
    camera.position.y =
      THREE.MathUtils.lerp(0.1, -0.28, approach) - pointerCurrent.y * 0.12
    earthGroup.position.x = THREE.MathUtils.lerp(3.55, 2.92, approach)
    earthGroup.position.y = THREE.MathUtils.lerp(-4.45, -3.96, approach)
    earthGroup.rotation.z = -0.16 + pointerCurrent.x * 0.018
    camera.lookAt(0, -0.45, 0)

    orbitGuides.rotation.z = elapsed * 0.014 + orbit * 0.08
    orbitGuides.children.forEach((child, index) => {
      const material = (child as THREE.LineLoop)
        .material as THREE.LineBasicMaterial
      const baseOpacity = material.userData.baseOpacity as number
      material.color.copy(darkTrace).lerp(lightTrace, appearanceMix)
      material.opacity = reveal * (0.35 + orbit * 0.65) * baseOpacity
      child.rotation.z = elapsed * (index === 0 ? 0.008 : -0.011)
    })

    for (const path of network.paths) {
      const lineMaterial = path.line.material as THREE.LineBasicMaterial
      const pulseMaterial = path.pulse.material as THREE.MeshBasicMaterial
      lineMaterial.color.copy(darkTrace).lerp(lightTrace, appearanceMix)
      pulseMaterial.color.copy(darkPulse).lerp(lightPulse, appearanceMix)
      lineMaterial.opacity =
        networkStrength * THREE.MathUtils.lerp(0.24, 0.3, appearanceMix)
      pulseMaterial.opacity = networkStrength * 0.68
      path.pulse.visible = networkStrength > 0.01
      path.curve.getPoint(
        (elapsed * path.speed + path.offset) % 1,
        path.pulse.position
      )
      const pulseScale = 0.9 + Math.sin(elapsed * 3.6 + path.offset * 10) * 0.2
      path.pulse.scale.setScalar(pulseScale)
    }

    renderer.render(scene, camera)
    if (!readyReported) {
      readyReported = true
      options.onReady()
    }
  }

  const startLoop = () => {
    if (running || disposed || !sceneVisible || document.hidden) return
    running = true
    lastFrameTime = performance.now()
    previousRenderTime = lastFrameTime
    animationFrame = window.requestAnimationFrame(renderFrame)
  }

  const stopLoop = () => {
    running = false
    window.cancelAnimationFrame(animationFrame)
    animationFrame = 0
  }

  const visibilityObserver = new IntersectionObserver(
    ([entry]) => {
      sceneVisible = entry.isIntersecting
      if (sceneVisible) startLoop()
      else stopLoop()
    },
    { threshold: 0.01 }
  )

  const resizeObserver = new ResizeObserver(resize)
  const onVisibilityChange = () => {
    if (document.hidden) stopLoop()
    else startLoop()
  }

  const onAbort = () => disposeRuntime()

  function disposeRuntime(releaseContext = true): void {
    if (disposed) return
    disposed = true
    stopLoop()
    resizeObserver.disconnect()
    visibilityObserver.disconnect()
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('scroll', onViewportShift)
    options.signal.removeEventListener('abort', onAbort)
    options.canvas.removeEventListener('webglcontextlost', onContextLost)
    scene.traverse((object) => {
      if (
        !(
          object instanceof THREE.Mesh ||
          object instanceof THREE.Line ||
          object instanceof THREE.Points
        )
      ) {
        return
      }
      object.geometry.dispose()
      const materials = Array.isArray(object.material)
        ? object.material
        : [object.material]
      for (const material of materials) material.dispose()
    })
    dayMap.dispose()
    nightMap.dispose()
    cloudMap.dispose()
    renderer.renderLists.dispose()
    renderer.dispose()
    if (releaseContext) renderer.forceContextLoss()
  }

  function onContextLost(event: Event): void {
    event.preventDefault()
    if (disposed) return
    disposeRuntime(false)
    options.onContextLost()
  }

  resize()
  resizeObserver.observe(options.container)
  visibilityObserver.observe(options.container)
  document.addEventListener('visibilitychange', onVisibilityChange)
  window.addEventListener('pointermove', onPointerMove, { passive: true })
  window.addEventListener('scroll', onViewportShift, { passive: true })
  options.signal.addEventListener('abort', onAbort, { once: true })
  options.canvas.addEventListener('webglcontextlost', onContextLost)
  if (options.signal.aborted) {
    disposeRuntime()
    return () => undefined
  }
  startLoop()

  return () => disposeRuntime()
}
