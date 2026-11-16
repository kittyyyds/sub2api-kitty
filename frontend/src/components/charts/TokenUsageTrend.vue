<template>
  <div class="card dashboard-chart-panel dashboard-token-trend p-4" data-dashboard-chart="token-trend">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.tokenUsageTrend') }}
    </h3>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="trendData.length > 0 && chartData" class="h-48">
      <Line :data="chartData" :options="lineOptions" />
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  type ChartOptions,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useDocumentDarkMode } from '@/composables/useDocumentDarkMode'
import { detectConsolePerformanceMode } from '@/utils/consolePerformance'
import type { TrendDataPoint } from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
  variant?: 'default' | 'customer'
}>()

const { isDarkMode } = useDocumentDarkMode()
const performanceMode = detectConsolePerformanceMode()

const chartColors = computed(() => {
  if (props.variant === 'customer') {
    return isDarkMode.value
      ? {
          text: '#b9c7c4',
          grid: 'rgba(224, 235, 232, 0.12)',
          input: '#8eeaff',
          output: '#84d9b0',
          cacheCreation: '#ffad68',
          cacheRead: '#a2b2b6',
          cacheHitRate: '#5dbfd0'
        }
      : {
          text: '#42545a',
          grid: 'rgba(20, 33, 38, 0.12)',
          input: '#0a7f90',
          output: '#28765f',
          cacheCreation: '#9c641f',
          cacheRead: '#60757d',
          cacheHitRate: '#177d9a'
        }
  }

  return {
    text: isDarkMode.value ? '#e6e2da' : '#353932',
    grid: isDarkMode.value ? '#353932' : '#e6e2da',
    input: '#3b82f6',
    output: '#10b981',
    cacheCreation: '#f59e0b',
    cacheRead: '#06b6d4',
    cacheHitRate: '#8b5cf6'
  }
})

const chartData = computed(() => {
  if (!props.trendData?.length) return null

  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: 'Input',
        data: props.trendData.map((d) => d.input_tokens),
        borderColor: chartColors.value.input,
        backgroundColor: `${chartColors.value.input}20`,
        fill: true,
        tension: 0.3
      },
      {
        label: 'Output',
        data: props.trendData.map((d) => d.output_tokens),
        borderColor: chartColors.value.output,
        backgroundColor: `${chartColors.value.output}20`,
        fill: true,
        tension: 0.3
      },
      {
        label: 'Cache Creation',
        data: props.trendData.map((d) => d.cache_creation_tokens),
        borderColor: chartColors.value.cacheCreation,
        backgroundColor: `${chartColors.value.cacheCreation}20`,
        borderDash: props.variant === 'customer' ? [10, 6] : undefined,
        fill: true,
        tension: 0.3
      },
      {
        label: 'Cache Read',
        data: props.trendData.map((d) => d.cache_read_tokens),
        borderColor: chartColors.value.cacheRead,
        backgroundColor: `${chartColors.value.cacheRead}20`,
        borderDash: props.variant === 'customer' ? [3, 4] : undefined,
        fill: true,
        tension: 0.3
      },
      {
        label: 'Cache Hit Rate',
        data: props.trendData.map((d) => {
          return d.cache_hit_rate == null ? null : d.cache_hit_rate * 100
        }),
        borderColor: chartColors.value.cacheHitRate,
        backgroundColor: `${chartColors.value.cacheHitRate}20`,
        borderDash: props.variant === 'customer' ? [2, 3, 8, 3] : [5, 5],
        fill: false,
        tension: 0.3,
        yAxisID: 'yPercent'
      }
    ]
  }
})

const lineOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: performanceMode === 'lite' ? false : { duration: 420 },
  animations: performanceMode === 'lite' ? {} : undefined,
  elements: {
    point: {
      radius: performanceMode === 'lite' ? 1.5 : 2.5,
      hoverRadius: performanceMode === 'lite' ? 3 : 4,
    },
    line: {
      borderWidth: performanceMode === 'lite' ? 1.25 : 1.8,
    },
  },
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      backgroundColor:
        props.variant === 'customer'
          ? isDarkMode.value
            ? 'rgba(6, 16, 20, 0.96)'
            : 'rgba(20, 33, 38, 0.96)'
          : undefined,
      titleColor: props.variant === 'customer' ? '#f2f7f5' : undefined,
      bodyColor: props.variant === 'customer' ? '#f2f7f5' : undefined,
      borderColor: props.variant === 'customer' ? chartColors.value.input : undefined,
      borderWidth: props.variant === 'customer' ? 1 : 0,
      callbacks: {
        label: (context: any) => {
          if (context.dataset.yAxisID === 'yPercent') {
            return `${context.dataset.label}: ${context.raw.toFixed(1)}%`
          }
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
        },
        footer: (tooltipItems: any) => {
          const dataIndex = tooltipItems[0]?.dataIndex
          if (dataIndex !== undefined && props.trendData[dataIndex]) {
            const data = props.trendData[dataIndex]
            return `Actual: $${formatCost(data.actual_cost)} | Standard: $${formatCost(data.cost)}`
          }
          return ''
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    },
    yPercent: {
      position: 'right' as const,
      min: 0,
      max: 100,
      grid: {
        drawOnChartArea: false
      },
      ticks: {
        color: chartColors.value.cacheHitRate,
        font: {
          size: 10
        },
        callback: (value: string | number) => `${value}%`
      }
    }
  }
}))

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  } else if (value >= 1) {
    return value.toFixed(2)
  } else if (value >= 0.01) {
    return value.toFixed(3)
  }
  return value.toFixed(4)
}
</script>
