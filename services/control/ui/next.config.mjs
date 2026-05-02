/** @type {import('next').NextConfig} */
const proxyTargetRaw = process.env.API_PROXY_TARGET || process.env.NEXT_PUBLIC_API_BASE || ""
const AUTO_TARGETS = [
  "http://127.0.0.1:8080",
  "http://localhost:8080",
  "http://127.0.0.1:8081",
  "http://localhost:8081",
]

const isAuto = (value) => {
  const v = String(value || "").trim()
  return !v || /^auto$/i.test(v)
}

const normalizeTarget = (value) => {
  const v = String(value || "").trim()
  if (!v) return ""
  if (v.startsWith("/")) return ""
  if (v.startsWith("http://") || v.startsWith("https://")) return v.replace(/\/+$/, "")
  return `http://${v}`.replace(/\/+$/, "")
}

const isLocalTarget = (target) => {
  try {
    const u = new URL(target)
    return u.hostname === "localhost" || u.hostname === "127.0.0.1"
  } catch {
    return false
  }
}

const probeTarget = async (target) => {
  if (!target) return false
  try {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), 600)
    const res = await fetch(`${target}/api/public/settings`, { signal: controller.signal })
    clearTimeout(timer)
    return res.ok
  } catch {
    return false
  }
}

const resolveProxyTarget = async () => {
  const normalized = isAuto(proxyTargetRaw) ? "" : normalizeTarget(proxyTargetRaw)
  if (normalized) {
    if (!isLocalTarget(normalized)) return normalized
    if (await probeTarget(normalized)) return normalized
  }
  for (const candidate of AUTO_TARGETS) {
    if (await probeTarget(candidate)) return candidate
  }
  return normalized || ""
}

const nextConfig = {
  images: {
    unoptimized: true,
  },
  reactStrictMode: false,
  compress: true,
  poweredByHeader: false,
  async rewrites() {
    const target = await resolveProxyTarget()
    if (!target) return []
    return [{ source: "/api/:path*", destination: `${target}/api/:path*` }]
  },
  compiler: {
    removeConsole: process.env.NODE_ENV === 'production',
  },
};

export default nextConfig;
