<template>
  <div
    ref="chartEl"
    class="traffic-map"
    :style="{ height: `${height}px` }"
    role="img"
    aria-label="Traffic map"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue"
import * as echarts from "echarts/core"
import type { EChartsType } from "echarts/core"
import { MapChart } from "echarts/charts"
import { GeoComponent, VisualMapComponent, TooltipComponent, GraphicComponent } from "echarts/components"
import { CanvasRenderer } from "echarts/renderers"
import worldGeoJSON from "@highcharts/map-collection/custom/world.geo.json"
import type { OverviewTrafficRegion } from "@/lib/api"
import { useTheme } from "@/composables/useTheme"
import { getChartPalette } from "@/lib/chartTheme"

echarts.use([MapChart, GeoComponent, VisualMapComponent, TooltipComponent, GraphicComponent, CanvasRenderer])

if (!echarts.getMap("world")) {
  echarts.registerMap("world", worldGeoJSON as any)
}

const props = defineProps<{
  regions: OverviewTrafficRegion[]
  height?: number
}>()

const chartEl = ref<HTMLDivElement | null>(null)
const height = computed(() => Math.max(220, props.height ?? 320))
const { isDark } = useTheme()

let chart: EChartsType | null = null
let resizeObserver: ResizeObserver | null = null

const toLocaleNumber = (value: any) => {
  const num = Number(value || 0)
  return Number.isFinite(num) ? num.toLocaleString("zh-CN") : "0"
}

const formatBytes = (bytes: number) => {
  if (bytes >= 1e12) return (bytes / 1e12).toFixed(1) + " TB"
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + " GB"
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + " MB"
  if (bytes >= 1e3) return (bytes / 1e3).toFixed(1) + " KB"
  return bytes + " B"
}

const toSeriesData = () =>
  props.regions
    .filter((r) => r.name && r.bytes_sent > 0)
    .map((r) => ({
      name: r.name,
      value: Number(r.bytes_sent || 0),
      requests: Number(r.requests || 0),
      region: r.region,
    }))

const maxValue = computed(() =>
  props.regions.reduce((acc, cur) => Math.max(acc, Number(cur.bytes_sent || 0)), 1)
)

const renderChart = () => {
  if (!chart) return

  const data = toSeriesData()
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
        if (!params?.data) return ""
        const d = params.data
        return [
          `<b>${d.name}</b>`,
          `流量: ${formatBytes(d.value)}`,
          d.requests > 0 ? `请求: ${toLocaleNumber(d.requests)}` : "",
        ]
          .filter(Boolean)
          .join("<br/>")
      },
    },
    visualMap: {
      min: 0,
      max: Math.max(maxValue.value, 1),
      left: 20,
      bottom: 20,
      text: ["高", "低"],
      textStyle: { color: palette.textFaint, fontSize: 11 },
      inRange: {
        color: palette.mapGradient,
      },
      calculable: true,
      itemWidth: 12,
      itemHeight: 100,
    },
    series: [
      {
        name: "访问流量",
        type: "map",
        map: "world",
        roam: true,
        zoom: 1.08,
        scaleLimit: { min: 1, max: 8 },
        layoutCenter: ["50%", "52%"],
        layoutSize: "118%",
        label: { show: false },
        itemStyle: {
          areaColor: palette.mapArea,
          borderColor: palette.mapBorder,
          borderWidth: 0.8,
        },
        emphasis: {
          label: { show: false },
          itemStyle: {
            areaColor: palette.mapAreaHover,
            borderColor: palette.brand,
            borderWidth: 1,
          },
        },
        data,
      },
    ],
    graphic:
      data.length > 0
        ? []
        : [
            {
              type: "text",
              left: "center",
              top: "middle",
              silent: true,
              style: {
                text: "暂无流量数据",
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
  renderChart()

  resizeObserver = new ResizeObserver(() => {
    chart?.resize()
  })
  resizeObserver.observe(chartEl.value)
})

watch(
  () => props.regions,
  () => renderChart(),
  { deep: true }
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
export default { name: "TrafficMap" }
</script>

