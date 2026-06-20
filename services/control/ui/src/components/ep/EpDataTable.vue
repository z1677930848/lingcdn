<template>
  <div class="ep-data-table">
    <el-table
      ref="tableRef"
      v-loading="loading"
      :data="pagedData"
      :row-key="rowKey"
      :size="size"
      :stripe="stripe"
      :empty-text="emptyText"
      class="ep-data-table__inner"
      @selection-change="onSelectionChange"
      @row-click="onRowClick"
    >
      <el-table-column
        v-for="col in selectionColumns"
        :key="col.colKey"
        type="selection"
        :width="col.width"
        :fixed="col.fixed"
        :reserve-selection="reserveSelection"
      />
      <el-table-column
        v-for="col in dataColumns"
        :key="col.colKey"
        :prop="col.colKey"
        :label="col.title"
        :width="col.width"
        :min-width="col.minWidth"
        :fixed="col.fixed"
        :show-overflow-tooltip="col.ellipsis !== false"
      >
        <template #default="{ row }">
          <CellRender v-if="col.cell" :render="col.cell" :row="row" />
          <span v-else>{{ formatCell(row, col.colKey) }}</span>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      v-if="paginationEnabled"
      class="ep-data-table__pagination"
      background
      layout="total, sizes, prev, pager, next, jumper"
      :total="paginationTotal"
      :current-page="currentPage"
      :page-size="pageSize"
      :page-sizes="pageSizeOptions"
      @current-change="onPageChange"
      @size-change="onSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch, type PropType } from "vue"
import type { TableInstance } from "element-plus"
import type { EpPaginationInput, EpTableColumn } from "./ep-table-types"

export type { EpPaginationConfig, EpPaginationInput, EpTableColumn } from "./ep-table-types"

const props = withDefaults(
  defineProps<{
    data?: unknown[]
    columns?: EpTableColumn[]
    rowKey?: string
    size?: "large" | "default" | "small"
    loading?: boolean
    empty?: string
    emptyText?: string
    pagination?: EpPaginationInput
    stripe?: boolean
    hover?: boolean
    selectedRowKeys?: Array<string | number>
    reserveSelection?: boolean
  }>(),
  {
    data: () => [],
    columns: () => [],
    rowKey: "id",
    size: "small",
    loading: false,
    empty: "暂无数据",
    stripe: false,
    selectedRowKeys: () => [],
    reserveSelection: true,
  }
)

const emit = defineEmits<{
  (e: "select-change", keys: Array<string | number>, context?: { selectedRowData?: unknown[] }): void
  (e: "row-click", ctx: { row: unknown }): void
  (e: "page-change", pagination: { current: number; pageSize: number }): void
}>()

const tableRef = ref<TableInstance>()
const syncingSelection = ref(false)

const CellRender = defineComponent({
  props: {
    render: { type: Function as PropType<EpTableColumn["cell"]>, required: true },
    row: { type: Object as PropType<Record<string, unknown>>, required: true },
  },
  setup(p) {
    return () => (p.render ? p.render(h, { row: p.row }) : null)
  },
})

const selectionColumns = computed(() => props.columns.filter((col) => col.type === "multiple"))
const dataColumns = computed(() => props.columns.filter((col) => col.type !== "multiple"))

const emptyText = computed(() => props.emptyText || props.empty || "暂无数据")

const paginationEnabled = computed(() => props.pagination !== false && props.pagination != null)

const pageSizeOptions = computed(() => {
  const p = props.pagination
  if (!p || typeof p === "boolean") return [10, 20, 50, 100]
  return p.pageSizeOptions || [10, 20, 50, 100]
})

const currentPage = ref(1)
const pageSize = ref(10)

watch(
  () => props.pagination,
  (p) => {
    if (!p || typeof p === "boolean") return
    currentPage.value = p.current ?? p.defaultCurrent ?? 1
    pageSize.value = p.pageSize ?? p.defaultPageSize ?? 10
  },
  { immediate: true, deep: true }
)

const paginationTotal = computed(() => {
  const p = props.pagination
  if (!p || typeof p === "boolean") return props.data.length
  return p.total ?? props.data.length
})

const pagedData = computed(() => {
  if (!paginationEnabled.value) return props.data
  const p = props.pagination
  if (p && typeof p !== "boolean" && (p.current != null || p.onChange)) {
    return props.data
  }
  const start = (currentPage.value - 1) * pageSize.value
  return props.data.slice(start, start + pageSize.value)
})

watch(
  () => [props.selectedRowKeys, props.data] as const,
  async () => {
    if (!selectionColumns.value.length) return
    await nextTick()
    const table = tableRef.value
    if (!table) return
    syncingSelection.value = true
    table.clearSelection()
    const keySet = new Set(props.selectedRowKeys)
    for (const row of props.data) {
      const record = row as Record<string, unknown>
      const key = record[props.rowKey] as string | number
      if (keySet.has(key)) {
        table.toggleRowSelection(row as Record<string, unknown>, true)
      }
    }
    syncingSelection.value = false
  },
  { deep: true, immediate: true }
)

function onPageChange(page: number) {
  currentPage.value = page
  const payload = { current: page, pageSize: pageSize.value }
  const p = props.pagination
  if (p && typeof p !== "boolean") {
    p.onChange?.(payload)
  }
  emit("page-change", payload)
}

function onSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  const payload = { current: 1, pageSize: size }
  const p = props.pagination
  if (p && typeof p !== "boolean") {
    p.onChange?.(payload)
  }
  emit("page-change", payload)
}

function onSelectionChange(rows: unknown[]) {
  if (syncingSelection.value) return
  const keys = rows.map((row) => (row as Record<string, unknown>)[props.rowKey] as string | number)
  emit("select-change", keys, { selectedRowData: rows })
}

function onRowClick(row: unknown) {
  emit("row-click", { row })
}

function formatCell(row: Record<string, unknown>, key: string) {
  const value = row[key]
  if (value == null || value === "") return "-"
  return String(value)
}
</script>

<style scoped>
.ep-data-table {
  width: 100%;
  overflow-x: auto;
}

.ep-data-table__inner {
  width: 100%;
  min-width: 960px;
}

.ep-data-table__pagination {
  margin-top: 16px;
  justify-content: flex-end;
}
</style>
