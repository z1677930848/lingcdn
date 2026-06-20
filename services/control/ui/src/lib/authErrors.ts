const AUTH_ERROR_MESSAGES: Record<string, string> = {
  "invalid credentials": "用户名或密码错误",
  unauthorized: "未授权，请重新登录",
  "user not found": "用户不存在",
  "invalid password": "密码错误",
  "account disabled": "账号已被禁用",
  "account locked": "账号已被锁定",
  "admin only": "需要管理员权限",
  "captcha required": "请完成下方验证码后再次登录",
  "captcha invalid": "验证码无效，请刷新后重试",
  "captcha expired": "验证码已过期，请刷新后重试",
  "captcha mismatch": "验证码错误，请刷新后重试",
  "请输入验证码": "请完成下方验证码后再次登录",
  "验证码不正确": "验证码错误，请刷新后重试",
  "验证码无效或已过期": "验证码已过期，请刷新后重试",
  "用户名或密码错误": "用户名或密码错误",
  "too many login attempts, try again later": "登录尝试过于频繁，请稍后再试",
  "too many requests": "请求过于频繁，请稍后再试",
}

export function isCaptchaAuthError(raw: string): boolean {
  const text = String(raw || "").trim().toLowerCase()
  if (!text) return false
  return (
    text.includes("captcha") ||
    text.includes("验证码") ||
    text === "请输入验证码"
  )
}

export function resolveAuthErrorMessage(raw: string, fallback: string): string {
  const text = String(raw || "").trim()
  if (!text) return fallback

  const direct = AUTH_ERROR_MESSAGES[text] || AUTH_ERROR_MESSAGES[text.toLowerCase()]
  if (direct) return direct

  if (isCaptchaAuthError(text)) {
    if (text.includes("过期") || text.includes("无效")) {
      return "验证码已过期，请刷新后重试"
    }
    if (text.includes("不正确") || text.includes("错误")) {
      return "验证码错误，请刷新后重试"
    }
    return "请完成下方验证码后再次登录"
  }

  return fallback
}
