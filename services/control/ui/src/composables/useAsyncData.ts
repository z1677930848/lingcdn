import { ref, onMounted } from "vue"
import type { Ref } from "vue"

interface UseAsyncDataOptions<T> {
  immediate?: boolean
  initialData?: T
  onSuccess?: (data: T) => void
  onError?: (error: Error) => void
}

interface UseAsyncDataReturn<T> {
  data: Ref<T | undefined>
  loading: Ref<boolean>
  error: Ref<string>
  execute: () => Promise<void>
  refresh: () => Promise<void>
}

export function useAsyncData<T>(
  fetcher: () => Promise<T>,
  options: UseAsyncDataOptions<T> = {},
): UseAsyncDataReturn<T> {
  const { immediate = true, initialData, onSuccess, onError } = options

  const data = ref<T | undefined>(initialData) as Ref<T | undefined>
  const loading = ref(false)
  const error = ref("")
  let executeSeq = 0

  async function execute() {
    const seq = ++executeSeq
    loading.value = true
    error.value = ""

    try {
      const result = await fetcher()
      if (seq === executeSeq) {
        data.value = result
        onSuccess?.(result)
      }
    } catch (err: unknown) {
      if (seq === executeSeq) {
        const message = err instanceof Error ? err.message : "加载数据失败"
        error.value = message
        onError?.(err instanceof Error ? err : new Error(message))
      }
    } finally {
      if (seq === executeSeq) {
        loading.value = false
      }
    }
  }

  if (immediate) {
    onMounted(execute)
  }

  return {
    data,
    loading,
    error,
    execute,
    refresh: execute,
  }
}
