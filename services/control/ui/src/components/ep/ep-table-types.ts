export type EpTableColumn = {
  colKey: string
  title?: string
  type?: string
  width?: number | string
  minWidth?: number | string
  fixed?: boolean | "left" | "right" | string
  ellipsis?: boolean
  cell?: (...args: any[]) => any
}

export type EpPaginationConfig = {
  current?: number
  pageSize?: number
  defaultCurrent?: number
  defaultPageSize?: number
  total?: number
  showJumper?: boolean
  showPageSize?: boolean
  pageSizeOptions?: number[]
  onChange?: (info: { current: number; pageSize: number }) => void
}

export type EpPaginationInput = boolean | EpPaginationConfig
