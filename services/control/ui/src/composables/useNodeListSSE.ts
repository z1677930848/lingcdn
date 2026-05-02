import { ref, onUnmounted } from "vue"
import type { MonitorSSEPayload } from "./useMonitorSSE"

export interface NodeMetricsSnapshot {
  node_id: string
  hostname: string
  cpu_usage: number
  mem_usage: number
  disk_usage: number
  out_bps: number
  in_bps: number
  connections: number
}

/**
 * 复用现有 /api/nodes/monitor/stream SSE 流，
 * 将 rank 数据转换为按 node_id 索引的指标 Map，
 * 供节点列表页合并刷新。
 */
export function useNodeListSSE() {
  const metricsMap = ref<Record<string, NodeMetricsSnapshot>>({})
  const connected = ref(false)

  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  function connect() {
    if (es) return
    es = new EventSource("/api/nodes/monitor/stream")

    es.addEventListener("monitor", (e: MessageEvent) => {
      try {
        const payload: MonitorSSEPayload = JSON.parse(e.data)
        connected.value = true
        if (payload.rank?.nodes) {
          const m: Record<string, NodeMetricsSnapshot> = {}
          for (const n of payload.rank.nodes) {
            m[n.node_id] = {
              node_id: n.node_id,
              hostname: n.hostname,
              cpu_usage: n.cpu_usage ?? 0,
              mem_usage: n.mem_usage ?? 0,
              disk_usage: n.disk_usage ?? 0,
              out_bps: n.out_bps ?? 0,
              in_bps: n.in_bps ?? 0,
              connections: n.connections ?? 0,
            }
          }
          metricsMap.value = m
        }
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

  return { metricsMap, connected, connect, disconnect }
}
