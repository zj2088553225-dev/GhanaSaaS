export const TURNSTILE_SCRIPT_URL = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'

export interface TurnstileRenderOptions {
  sitekey: string
  theme: 'auto'
  size: 'flexible'
  appearance: 'interaction-only'
  action: 'reservation_otp' | 'reservation_create'
  callback: (token: string) => void
  'expired-callback': () => void
  'error-callback': () => void
}

export interface TurnstileAPI {
  render(container: HTMLElement, options: TurnstileRenderOptions): string
  reset(widgetID: string): void
  remove(widgetID: string): void
}

export interface ReservationTurnstileCallbacks {
  success(token: string): void
  expired(): void
  error(): void
}

export function reservationTurnstileOptions(
  siteKey: string,
  callbacks: ReservationTurnstileCallbacks,
  action: TurnstileRenderOptions['action'] = 'reservation_otp',
): TurnstileRenderOptions {
  return {
    sitekey: siteKey.trim(),
    theme: 'auto',
    size: 'flexible',
    appearance: 'interaction-only',
    action,
    callback: callbacks.success,
    'expired-callback': callbacks.expired,
    'error-callback': callbacks.error,
  }
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

let defaultLoaderPromise: Promise<TurnstileAPI> | null = null

function removeTurnstileScript(script: HTMLScriptElement) {
  if (typeof script.remove === 'function') {
    script.remove()
  } else if (script.parentNode) {
    script.parentNode.removeChild(script)
  }
}

export function loadTurnstileAPI(targetWindow: Window = window, targetDocument: Document = document): Promise<TurnstileAPI> {
  const isDefaultWindow = typeof window !== 'undefined' && targetWindow === window
  if (targetWindow.turnstile) {
    return Promise.resolve(targetWindow.turnstile)
  }
  if (isDefaultWindow && defaultLoaderPromise) {
    return defaultLoaderPromise
  }

  const promise = new Promise<TurnstileAPI>((resolve, reject) => {
    let settled = false
    let timeoutID: ReturnType<typeof setTimeout> | null = null
    const finish = (callback: () => void) => {
      if (settled) return
      settled = true
      if (timeoutID) clearTimeout(timeoutID)
      callback()
    }
    const resolveAPI = () => {
      if (targetWindow.turnstile) {
        finish(() => resolve(targetWindow.turnstile as TurnstileAPI))
      } else {
        finish(() => reject(new Error('Turnstile API did not initialize')))
      }
    }
    const rejectLoad = (script: HTMLScriptElement) => finish(() => {
      script.dataset.gcTurnstileState = 'failed'
      removeTurnstileScript(script)
      reject(new Error('Turnstile script failed to load'))
    })
    let existing = targetDocument.querySelector<HTMLScriptElement>('script[data-gc-turnstile]')
    if (existing?.dataset.gcTurnstileState === 'failed') {
      removeTurnstileScript(existing)
      existing = null
    }
    if (existing) {
      if (existing.dataset.gcTurnstileState === 'loaded') {
        resolveAPI()
        return
      }
      existing.addEventListener('load', resolveAPI, { once: true })
      existing.addEventListener('error', () => rejectLoad(existing as HTMLScriptElement), { once: true })
      timeoutID = setTimeout(() => rejectLoad(existing as HTMLScriptElement), 10_000)
      return
    }

    const script = targetDocument.createElement('script')
    script.src = TURNSTILE_SCRIPT_URL
    script.async = true
    script.defer = true
    script.dataset.gcTurnstile = 'true'
    script.dataset.gcTurnstileState = 'loading'
    script.addEventListener('load', () => {
      script.dataset.gcTurnstileState = 'loaded'
      resolveAPI()
    }, { once: true })
    script.addEventListener('error', () => rejectLoad(script), { once: true })
    targetDocument.head.appendChild(script)
    timeoutID = setTimeout(() => rejectLoad(script), 10_000)
  })

  if (isDefaultWindow) {
    defaultLoaderPromise = promise.catch((error) => {
      defaultLoaderPromise = null
      throw error
    })
    return defaultLoaderPromise
  }
  return promise
}

export function resetTurnstileLoaderForTests() {
  defaultLoaderPromise = null
}
