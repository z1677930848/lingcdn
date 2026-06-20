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
  const reconnecting = ref(false)

  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  // `disposed` guards against a race where the component unmounts after
  // the reconnect timer fires but before its callback re-enters
  // `connect()`. clearTimeout is a no-op once the callback has been
  // queued, so without this flag a stray EventSource could outlive the
  // composable and keep streaming bytes to a dead view.
  let disposed = false

  function connect() {
    if (disposed || es) return
    es = new EventSource("/api/nodes/monitor/stream")

    es.addEventListener("monitor", (e: MessageEvent) => {
      try {
        data.value = JSON.parse(e.data)
        connected.value = true
        reconnecting.value = false
      } catch {
        // ignore parse errors
      }
    })

    es.onopen = () => {
      connected.value = true
      reconnecting.value = false
    }

    es.onerror = () => {
      connected.value = false
      reconnecting.value = true
      es?.close()
      es = null
      if (!disposed) {
        reconnectTimer = setTimeout(connect, 5000)
      }
    }
  }

  function disconnect() {
    disposed = true
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    es?.close()
    es = null
    connected.value = false
    reconnecting.value = false
  }

  onUnmounted(disconnect)

  return { data, connected, reconnecting, connect, disconnect }
}
