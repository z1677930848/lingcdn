import { ref, onUnmounted } from "vue"
import type { NodeMonitorRankEntry, NodeMonitorSeriesResponse } from "@/lib/api"

export interface MonitorSSEPayload {
  rank: {
    group: string
    window_seconds: number
    nodes: NodeMonitorRankEntry[]
  }
  series: Record<string, NodeMonitorSeriesResponse>
}

export function useMonitorSSE() {
  const data = ref<MonitorSSEPayload | null>(null)
  const connected = ref(false)

  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  function connect() {
    if (es) return
    es = new EventSource("/api/nodes/monitor/stream")

    es.addEventListener("monitor", (e: MessageEvent) => {
      try {
        data.value = JSON.parse(e.data)
        connected.value = true
      } catch {
        // ignore parse errors
      }
    })

    es.onopen = () => {
      connected.value = true
    }

    es.onerror = () => {
      connected.value = false
      es?.close()
      es = null
      reconnectTimer = setTimeout(connect, 5000)
    }
  }

  function disconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    es?.close()
    es = null
    connected.value = false
  }

  onUnmounted(disconnect)

  return { data, connected, connect, disconnect }
}
