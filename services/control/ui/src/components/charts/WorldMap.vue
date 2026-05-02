<template>
  <div
    ref="chartEl"
    class="world-map"
    :style="{ height: `${height}px` }"
    role="img"
    aria-label="World map"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue"
import * as echarts from "echarts/core"
import type { EChartsType } from "echarts/core"
import { ScatterChart } from "echarts/charts"
import { GeoComponent, TooltipComponent } from "echarts/components"
import { CanvasRenderer } from "echarts/renderers"
import worldGeoJSON from "@highcharts/map-collection/custom/world.geo.json"
import { useTheme } from "@/composables/useTheme"
import { getChartPalette } from "@/lib/chartTheme"

echarts.use([ScatterChart, GeoComponent, TooltipComponent, CanvasRenderer])

if (!echarts.getMap("world")) {
  echarts.registerMap("world", worldGeoJSON as any)
}

type WorldMapPoint = {
  id: string
  name: string
  lat: number
  lon: number
  value: number
  total?: number
  dropped?: number
  nodes?: number
}

const props = defineProps<{
  points: WorldMapPoint[]
  selectedId?: string
  height?: number
}>()

const emit = defineEmits<{
  (e: "select", id: string): void
}>()

const chartEl = ref<HTMLDivElement | null>(null)
const height = computed(() => Math.max(220, props.height ?? 320))
const maxValue = computed(() => props.points.reduce((acc, cur) => Math.max(acc, Number(cur.value || 0)), 1))
const { isDark } = useTheme()

let chart: EChartsType | null = null
let resizeObserver: ResizeObserver | null = null

const toLocaleNumber = (value: any) => {
  const num = Number(value || 0)
  return Number.isFinite(num) ? num.toLocaleString("zh-CN") : "0"
}

const toSeriesData = () =>
  props.points.map((point) => ({
    id: point.id,
    name: point.name || point.id,
    value: [Number(point.lon || 0), Number(point.lat || 0), Number(point.value || 0)],
    total: Number(point.total || 0),
    dropped: Number(point.dropped || 0),
    nodes: Number(point.nodes || 0),
  }))

const renderChart = () => {
  if (!chart) return

  const palette = getChartPalette(isDark.value)

  const option: any = {
    backgroundColor: "transparent",
    tooltip: {
      trigger: "item",
      confine: true,
      backgroundColor: palette.surface,
      borderColor: palette.border,
      textStyle: { color: palette.textPrimary, fontSize: 12 },
      formatter: (params: any) => {
        const data = params?.data
        if (!data) return ""
        return [
          String(data.name || "-"),
          `Nodes: ${toLocaleNumber(data.nodes)}`,
          `Dropped: ${toLocaleNumber(data.dropped)}`,
          `Total: ${toLocaleNumber(data.total)}`,
        ].join("<br/>")
      },
    },
    geo: {
      map: "world",
      roam: true,
      zoom: 1.08,
      scaleLimit: { min: 1, max: 8 },
      itemStyle: {
        areaColor: palette.mapArea,
        borderColor: palette.mapBorder,
        borderWidth: 0.8,
      },
      emphasis: {
        itemStyle: {
          areaColor: palette.mapAreaHover,
        },
      },
      layoutCenter: ["50%", "52%"],
      layoutSize: "118%",
    },
    series: [
      {
        name: "Dropped",
        type: "scatter",
        coordinateSystem: "geo",
        zlevel: 2,
        data: toSeriesData(),
        symbolSize: (value: any, params: any) => {
          const dropped = Array.isArray(value) ? Number(value[2] || 0) : 0
          const ratio = Math.sqrt(Math.max(0, dropped) / Math.max(1, maxValue.value))
          const selected = String(params?.data?.id || "") === String(props.selectedId || "")
          return (selected ? 10 : 7) + ratio * 18
        },
        itemStyle: {
          color: (params: any) =>
            String(params?.data?.id || "") === String(props.selectedId || "") ? palette.brand : palette.brandSoft,
          borderColor: (params: any) =>
            String(params?.data?.id || "") === String(props.selectedId || "") ? palette.brandStrong : palette.brand,
          borderWidth: (params: any) => (String(params?.data?.id || "") === String(props.selectedId || "") ? 2 : 1),
          shadowBlur: 10,
          shadowColor: palette.shadow,
        },
        emphasis: {
          scale: true,
        },
      },
    ],
    graphic:
      props.points.length > 0
        ? []
        : [
            {
              type: "text",
              left: "center",
              top: "middle",
              silent: true,
              style: {
                text: "No geo data",
                fill: palette.textFaint,
                fontSize: 14,
                fontFamily: "sans-serif",
              },
            },
          ],
  }

  chart.setOption(option, true)
}

onMounted(() => {
  if (!chartEl.value) return
  chart = echarts.init(chartEl.value)
  chart.on("click", (params: any) => {
    if (params?.seriesType !== "scatter") return
    const id = String(params?.data?.id || "").trim()
    if (id) emit("select", id)
  })
  renderChart()

  resizeObserver = new ResizeObserver(() => {
    chart?.resize()
  })
  resizeObserver.observe(chartEl.value)
})

watch(
  () => props.points,
  () => renderChart(),
  { deep: true }
)

watch(
  () => props.selectedId,
  () => renderChart()
)

watch(height, () => {
  chart?.resize()
  renderChart()
})

watch(isDark, () => renderChart())

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (chart) {
    chart.dispose()
    chart = null
  }
})
</script>

<script lang="ts">
export default { name: "WorldMap" }
</script>

<style scoped>
.world-map {
  width: 100%;
  border-radius: var(--app-card-radius);
  overflow: hidden;
  background: var(--app-surface-muted);
  transition: background var(--app-anim-base) var(--app-easing-standard);
}
</style>
