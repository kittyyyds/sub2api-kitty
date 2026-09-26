import { onBeforeUnmount, onMounted, readonly, shallowRef, toValue, watch } from 'vue'
import type { MaybeRefOrGetter, ShallowRef } from 'vue'

const vertexShaderSource = `#version 300 es
  in vec2 aPosition;
  void main() {
    gl_Position = vec4(aPosition, 0.0, 1.0);
  }
`

const fragmentShaderSource = `#version 300 es
  precision highp float;

  uniform vec2 iResolution;
  uniform float iTime;
  uniform vec3 uColor1;
  uniform vec3 uColor2;
  uniform vec3 uColor3;
  uniform vec3 uColor4;
  uniform vec2 uPointer;
  uniform float uPointerStrength;
  out vec4 fragColor;

  float hash(vec2 p) {
    p = fract(p * vec2(123.34, 456.21));
    p += dot(p, p + 45.32);
    return fract(p.x * p.y);
  }

  float noise(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    f = f * f * (3.0 - 2.0 * f);
    float a = hash(i);
    float b = hash(i + vec2(1.0, 0.0));
    float c = hash(i + vec2(0.0, 1.0));
    float d = hash(i + vec2(1.0, 1.0));
    return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
  }

  float multiNoise(vec2 p) {
    return (noise(p) * 0.5 + noise(p * 2.0) * 0.25) * 1.06667;
  }

  void main() {
    float time = iTime * 0.34;
    vec2 uv = gl_FragCoord.xy / iResolution.x;
    vec2 pointer = vec2(uPointer.x, uPointer.y * iResolution.y / iResolution.x);
    vec2 pointerDelta = uv - pointer;
    float pointerFalloff = exp(-length(pointerDelta) * 8.0) * uPointerStrength;
    vec2 pointerNoise = vec2(
      noise(uv * 6.0 + vec2(time * 0.9, 0.0)),
      noise(uv * 6.0 + vec2(0.0, time * 1.1))
    ) - 0.5;
    uv += pointerNoise * pointerFalloff * 0.55;
    uv *= 4.2;

    vec2 d1 = vec2(0.18, 0.07) * time;
    vec2 d2 = vec2(-0.13, 0.21) * time;
    vec2 d3 = vec2(0.09, -0.16) * time;

    float a = multiNoise(uv + d1);
    vec3 color = mix(uColor1, uColor2, clamp(a * 2.5, 0.0, 1.0));
    float b = multiNoise(uv + a * 2.4 + d2);
    color = mix(color, uColor3, clamp(b * 1.45, 0.0, 1.0));
    float c = multiNoise(uv + b * 3.5 + d3);
    color = mix(color, uColor4, clamp(c * 0.95, 0.0, 1.0));
    color += mix(uColor4, uColor2, 0.45) * pointerFalloff * 0.10;

    float grain = fract(sin(dot(uv * 2.0, vec2(12.9898, 78.233))) * 43758.5453);
    color += (grain - 0.5) * 0.018;
    color = (color - 0.5) * 1.08 + 0.5;
    float luma = dot(color, vec3(0.2126, 0.7152, 0.0722));
    color = mix(vec3(luma), color, 1.1);
    fragColor = vec4(clamp(color, 0.0, 1.0), 1.0);
  }
`

const lightPalette = [
  [1, 0.8157, 0.7412],
  [1, 0.451, 0.3922],
  [1, 0.8275, 0.3098],
  [0.502, 0.7765, 0.949],
]

const darkPalette = [
  [0.1451, 0.2627, 0.5255],
  [0.5412, 0.2863, 0.6118],
  [0.9412, 0.4824, 0.3412],
  [0.1451, 0.7059, 0.7569],
]

const MAX_CANVAS_DPR = 1.25

interface FluidUniforms {
  resolution: WebGLUniformLocation | null
  time: WebGLUniformLocation | null
  colors: Array<WebGLUniformLocation | null>
  pointer: WebGLUniformLocation | null
  pointerStrength: WebGLUniformLocation | null
}

function createShader(gl: WebGL2RenderingContext, type: number, source: string): WebGLShader | null {
  const shader = gl.createShader(type)
  if (!shader) return null
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    console.warn('HeyRoute fluid shader failed:', gl.getShaderInfoLog(shader))
    gl.deleteShader(shader)
    return null
  }
  return shader
}

export function useHeyRouteFluidCanvas(
  canvasRef: ShallowRef<HTMLCanvasElement | null>,
  isDark: MaybeRefOrGetter<boolean>,
  isActive: MaybeRefOrGetter<boolean> = true,
) {
  const isReady = shallowRef(false)
  let gl: WebGL2RenderingContext | null = null
  let program: WebGLProgram | null = null
  let buffer: WebGLBuffer | null = null
  let uniforms: FluidUniforms | null = null
  let frameId = 0
  let resizeFrameId = 0
  let resizeObserver: ResizeObserver | null = null
  let intersectionObserver: IntersectionObserver | null = null
  let reduceMotion: MediaQueryList | null = null
  let finePointer: MediaQueryList | null = null
  let startTime = 0
  let previousTime = 0
  let isVisible = true
  let pageVisible = true
  let contextLost = false
  let pointerTargetX = 0.5
  let pointerTargetY = 0.5
  let pointerX = 0.5
  let pointerY = 0.5
  let pointerStrengthTarget = 0
  let pointerStrength = 0
  let currentPalette = lightPalette.map((color) => [...color])
  let targetPalette = lightPalette

  function resize(): boolean {
    const canvas = canvasRef.value
    if (!canvas || !gl || contextLost) return false
    const rect = canvas.getBoundingClientRect()
    const dpr = Math.min(window.devicePixelRatio || 1, MAX_CANVAS_DPR)
    const width = Math.max(1, Math.round(rect.width * dpr))
    const height = Math.max(1, Math.round(rect.height * dpr))
    if (canvas.width === width && canvas.height === height) return false
    isReady.value = false
    canvas.width = width
    canvas.height = height
    gl.viewport(0, 0, width, height)
    return true
  }

  function interpolateState(deltaMs: number): void {
    const frameScale = deltaMs / (1000 / 60)
    const pointerResponse = 1 - Math.pow(1 - 0.12, frameScale)
    const strengthResponse = 1 - Math.pow(1 - 0.18, frameScale)
    pointerX += (pointerTargetX - pointerX) * pointerResponse
    pointerY += (pointerTargetY - pointerY) * pointerResponse
    pointerStrength += (pointerStrengthTarget - pointerStrength) * strengthResponse

    currentPalette.forEach((color, colorIndex) => {
      color.forEach((value, channelIndex) => {
        color[channelIndex] = value + (targetPalette[colorIndex][channelIndex] - value) * strengthResponse
      })
    })
  }

  function draw(now: number, deltaMs = 1000 / 60): void {
    const canvas = canvasRef.value
    if (!canvas || !gl || !program || !uniforms || contextLost) return
    resize()
    interpolateState(deltaMs)
    gl.useProgram(program)
    gl.uniform2f(uniforms.resolution, canvas.width, canvas.height)
    gl.uniform1f(uniforms.time, (now - startTime) / 1000)
    uniforms.colors.forEach((location, index) => gl?.uniform3fv(location, currentPalette[index]))
    gl.uniform2f(uniforms.pointer, pointerX, pointerY)
    gl.uniform1f(uniforms.pointerStrength, pointerStrength)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    if (!gl.isContextLost()) isReady.value = true
  }

  function animate(now: number): void {
    if (contextLost || !pageVisible || !isVisible || reduceMotion?.matches || !toValue(isActive)) {
      frameId = 0
      return
    }
    const deltaMs = Math.min(Math.max(now - previousTime, 0), 50)
    previousTime = now
    draw(now, deltaMs)
    frameId = window.requestAnimationFrame(animate)
  }

  function startAnimation(): void {
    if (frameId || contextLost || !pageVisible || reduceMotion?.matches || !isVisible || !toValue(isActive)) return
    previousTime = performance.now()
    frameId = window.requestAnimationFrame(animate)
  }

  function stopAnimation(): void {
    window.cancelAnimationFrame(frameId)
    frameId = 0
  }

  function scheduleResizeRender(): void {
    if (resizeFrameId || contextLost || !pageVisible) return
    isReady.value = false
    resizeFrameId = window.requestAnimationFrame((now) => {
      resizeFrameId = 0
      draw(now)
    })
  }

  function handleContextLost(): void {
    contextLost = true
    isReady.value = false
    stopAnimation()
    window.cancelAnimationFrame(resizeFrameId)
    resizeFrameId = 0
  }

  function handlePointerMove(event: PointerEvent): void {
    const canvas = canvasRef.value
    if (!canvas || !finePointer?.matches || event.pointerType === 'touch') return
    const rect = canvas.getBoundingClientRect()
    const inside = event.clientX >= rect.left && event.clientX <= rect.right
      && event.clientY >= rect.top && event.clientY <= rect.bottom
    if (!inside) {
      pointerStrengthTarget = 0
      return
    }
    pointerTargetX = (event.clientX - rect.left) / Math.max(rect.width, 1)
    pointerTargetY = 1 - (event.clientY - rect.top) / Math.max(rect.height, 1)
    pointerStrengthTarget = 1
  }

  function handlePointerLeave(): void {
    pointerStrengthTarget = 0
  }

  function handleVisibilityChange(): void {
    pageVisible = !document.hidden
    if (!pageVisible) {
      stopAnimation()
      return
    }
    scheduleResizeRender()
    syncMotionPreference()
  }

  function renderStaticFrame(): void {
    stopAnimation()
    pointerStrength = 0
    pointerStrengthTarget = 0
    currentPalette = targetPalette.map((color) => [...color])
    draw(startTime + 1750, 1000 / 60)
  }

  function syncMotionPreference(): void {
    if (!toValue(isActive)) {
      stopAnimation()
      return
    }
    if (reduceMotion?.matches) {
      renderStaticFrame()
      return
    }
    startAnimation()
  }

  onMounted(() => {
    const canvas = canvasRef.value
    if (!canvas) return

    gl = canvas.getContext('webgl2', {
      alpha: true,
      antialias: false,
      depth: false,
      powerPreference: 'high-performance',
      preserveDrawingBuffer: false,
    })
    if (!gl) return

    const vertexShader = createShader(gl, gl.VERTEX_SHADER, vertexShaderSource)
    const fragmentShader = createShader(gl, gl.FRAGMENT_SHADER, fragmentShaderSource)
    if (!vertexShader || !fragmentShader) return

    program = gl.createProgram()
    if (!program) return
    gl.attachShader(program, vertexShader)
    gl.attachShader(program, fragmentShader)
    gl.linkProgram(program)
    gl.deleteShader(vertexShader)
    gl.deleteShader(fragmentShader)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      console.warn('HeyRoute fluid program failed:', gl.getProgramInfoLog(program))
      return
    }

    buffer = gl.createBuffer()
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW)
    gl.useProgram(program)
    const position = gl.getAttribLocation(program, 'aPosition')
    gl.enableVertexAttribArray(position)
    gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0)

    uniforms = {
      resolution: gl.getUniformLocation(program, 'iResolution'),
      time: gl.getUniformLocation(program, 'iTime'),
      colors: [1, 2, 3, 4].map((index) => gl?.getUniformLocation(program as WebGLProgram, `uColor${index}`) ?? null),
      pointer: gl.getUniformLocation(program, 'uPointer'),
      pointerStrength: gl.getUniformLocation(program, 'uPointerStrength'),
    }

    targetPalette = toValue(isDark) ? darkPalette : lightPalette
    currentPalette = targetPalette.map((color) => [...color])
    startTime = performance.now()
    previousTime = startTime
    pageVisible = !document.hidden
    reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
    finePointer = window.matchMedia('(hover: hover) and (pointer: fine)')
    resizeObserver = new ResizeObserver(scheduleResizeRender)
    resizeObserver.observe(canvas)
    intersectionObserver = new IntersectionObserver(([entry]) => {
      isVisible = entry?.isIntersecting ?? true
      if (isVisible) startAnimation()
      else stopAnimation()
    }, { threshold: 0.01 })
    intersectionObserver.observe(canvas)
    window.addEventListener('pointermove', handlePointerMove, { passive: true })
    document.documentElement.addEventListener('pointerleave', handlePointerLeave, { passive: true })
    window.addEventListener('blur', handlePointerLeave)
    document.addEventListener('visibilitychange', handleVisibilityChange)
    canvas.addEventListener('webglcontextlost', handleContextLost)
    reduceMotion.addEventListener('change', syncMotionPreference)
    resize()
    draw(startTime)
    syncMotionPreference()
  })

  watch(
    () => toValue(isDark),
    (dark) => {
      targetPalette = dark ? darkPalette : lightPalette
      if (reduceMotion?.matches) renderStaticFrame()
      else startAnimation()
    },
  )

  watch(
    () => toValue(isActive),
    (active) => {
      if (active) syncMotionPreference()
      else stopAnimation()
    },
  )

  onBeforeUnmount(() => {
    stopAnimation()
    window.cancelAnimationFrame(resizeFrameId)
    resizeFrameId = 0
    window.removeEventListener('pointermove', handlePointerMove)
    document.documentElement.removeEventListener('pointerleave', handlePointerLeave)
    window.removeEventListener('blur', handlePointerLeave)
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    reduceMotion?.removeEventListener('change', syncMotionPreference)
    resizeObserver?.disconnect()
    intersectionObserver?.disconnect()
    canvasRef.value?.removeEventListener('webglcontextlost', handleContextLost)
    if (gl && buffer) gl.deleteBuffer(buffer)
    if (gl && program) gl.deleteProgram(program)
    gl?.getExtension('WEBGL_lose_context')?.loseContext()
    gl = null
    program = null
    buffer = null
    uniforms = null
  })

  return {
    isReady: readonly(isReady),
  }
}
