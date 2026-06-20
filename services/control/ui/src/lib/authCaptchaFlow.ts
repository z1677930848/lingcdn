import { isCaptchaAuthError, resolveAuthErrorMessage } from "@/lib/authErrors"
import type { NotifyVariant } from "@/lib/notify"

export async function notifyAuthFailure(options: {
  err: unknown
  fallback: string
  requireCaptcha: { value: boolean }
  captchaStorageKey: string
  loadCaptcha: (silent?: boolean) => Promise<void>
  notify: (payload: { variant?: NotifyVariant; title: string; description?: string }) => void
}) {
  const rawError = options.err instanceof Error ? options.err.message : String(options.err ?? options.fallback)
  const activatingCaptcha = !options.requireCaptcha.value
  const captchaTriggered = activatingCaptcha && isCaptchaAuthError(rawError)
  const title = resolveAuthErrorMessage(
    rawError,
    captchaTriggered ? "请完成下方验证码后再次登录" : options.fallback
  )

  options.notify({
    variant: captchaTriggered ? "warning" : "destructive",
    title,
  })

  if (activatingCaptcha) {
    options.requireCaptcha.value = true
    localStorage.setItem(options.captchaStorageKey, "true")
  }

  try {
    await options.loadCaptcha()
  } catch {
    // captcha endpoint may be unavailable in some deployments
  }
}
