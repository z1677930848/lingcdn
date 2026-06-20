import { ref, onUnmounted } from "vue"
import { api, type SyncTask } from "@/lib/api"

// useSyncTasks tracks per-subject sync state (publish + dns) for UI rows.
// It bootstraps from GET /api/sync/active, then keeps up to date via the
// shared SSE stream at /api/sync/stream (event: sync).
//
// Success tasks are kept for a short fade-out window so the "同步中" → done
// transition isn't a flash; failed tasks stay until superseded by a success
// or explicitly dismissed.
export function useSyncTasks(subjectPrefix?: string) {
  const tasksBySubject = ref<Record<string, SyncTask>>({})
  const successCleanupTimers = new Map<string, ReturnType<typeof setTimeout>>()

  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  function matchesPrefix(sub: string): boolean {
    if (!subjectPrefix) return true
    return sub.startsWith(subjectPrefix)
  }

  function upsert(task: SyncTask) {
    if (!task || !task.subject) return
    if (!matchesPrefix(task.subject)) return

    // Always take the latest event per subject. Subsequent "success"/"failed"
    // events from the same task close it out; a fresh "running" event from a
    // retry supersedes a prior "failed".
    tasksBySubject.value = { ...tasksBySubject.value, [task.subject]: task }

    const existingTimer = successCleanupTimers.get(task.subject)
    if (existingTimer) {
      clearTimeout(existingTimer)
      successCleanupTimers.delete(task.subject)
    }
    if (task.status === "success") {
      const t = setTimeout(() => {
        const cur = tasksBySubject.value[task.subject]
        if (cur && cur.id === task.id && cur.status === "success") {
          const next = { ...tasksBySubject.value }
          delete next[task.subject]
          tasksBySubject.value = next
        }
        successCleanupTimers.delete(task.subject)
      }, 3000)
      successCleanupTimers.set(task.subject, t)
    }
  }

  async function bootstrap() {
    try {
      const tasks = await api.listActiveSyncTasks(subjectPrefix)
      for (const t of tasks) upsert(t)
    } catch {
      // ignore — SSE will catch up
    }
  }

  function connect() {
    if (es) return
    es = new EventSource("/api/sync/stream")
    es.addEventListener("sync", (e: MessageEvent) => {
      try {
        const task = JSON.parse(e.data) as SyncTask
        upsert(task)
      } catch {
        // ignore
      }
    })
    es.onerror = () => {
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
    for (const t of successCleanupTimers.values()) clearTimeout(t)
    successCleanupTimers.clear()
  }

  function dismiss(subject: string) {
    const next = { ...tasksBySubject.value }
    delete next[subject]
    tasksBySubject.value = next
    const t = successCleanupTimers.get(subject)
    if (t) {
      clearTimeout(t)
      successCleanupTimers.delete(subject)
    }
  }

  onUnmounted(disconnect)

  return {
    tasksBySubject,
    bySubject: (s: string): SyncTask | null => tasksBySubject.value[s] || null,
    isSyncing: (s: string): boolean => tasksBySubject.value[s]?.status === "running",
    isFailed: (s: string): boolean => tasksBySubject.value[s]?.status === "failed",
    bootstrap,
    connect,
    disconnect,
    dismiss,
  }
}
