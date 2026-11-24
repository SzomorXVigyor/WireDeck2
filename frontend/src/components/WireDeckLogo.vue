<script setup lang="ts">
import { computed, useId } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    variant?: 'default' | 'admin' | 'operator'
  }>(),
  {
    size: 'md',
    variant: 'default',
  },
)

const uid = useId()
const baseId = computed(() => `wd-base-${uid}`)
const wireId = computed(() => `wd-wire-${uid}`)
const topId = computed(() => `wd-top-${uid}`)

const sizeClass = computed(() => {
  switch (props.size) {
    case 'sm':
      return 'h-7 w-7'
    case 'lg':
      return 'h-10 w-10'
    default:
      return 'h-8 w-8'
  }
})

const palette = computed(() => {
  switch (props.variant) {
    case 'admin':
      return {
        baseStart: '#4338ca',
        baseEnd: '#312e81',
        wireStart: '#a5b4fc',
        wireEnd: '#818cf8',
        topLight: '#c7d2fe',
        topMid: '#6366f1',
        topDark: '#4f46e5',
        coreDot: '#4338ca',
      }
    case 'operator':
      return {
        baseStart: '#b45309',
        baseEnd: '#78350f',
        wireStart: '#fde68a',
        wireEnd: '#fbbf24',
        topLight: '#fef08a',
        topMid: '#f59e0b',
        topDark: '#d97706',
        coreDot: '#b45309',
      }
    default:
      return {
        baseStart: '#0369a1',
        baseEnd: '#1e3a8a',
        wireStart: '#22d3ee',
        wireEnd: '#38bdf8',
        topLight: '#7dd3fc',
        topMid: '#0ea5e9',
        topDark: '#0284c7',
        coreDot: '#0284c7',
      }
  }
})
</script>

<template>
  <svg
    :class="[sizeClass, 'shrink-0 select-none']"
    viewBox="0 0 64 64"
    fill="none"
    role="img"
    aria-label="WireDeck logo"
  >
    <defs>
      <linearGradient :id="baseId" x1="8" y1="36" x2="56" y2="56" gradientUnits="userSpaceOnUse">
        <stop offset="0%" :stop-color="palette.baseStart" />
        <stop offset="100%" :stop-color="palette.baseEnd" />
      </linearGradient>
      <linearGradient :id="wireId" x1="10" y1="24" x2="54" y2="42" gradientUnits="userSpaceOnUse">
        <stop offset="0%" :stop-color="palette.wireStart" />
        <stop offset="100%" :stop-color="palette.wireEnd" />
      </linearGradient>
      <linearGradient :id="topId" x1="10" y1="8" x2="54" y2="30" gradientUnits="userSpaceOnUse">
        <stop offset="0%" :stop-color="palette.topLight" />
        <stop offset="45%" :stop-color="palette.topMid" />
        <stop offset="100%" :stop-color="palette.topDark" />
      </linearGradient>
    </defs>

    <!-- Bottom Layer: Remote Site Plane -->
    <path
      d="M30.2 35.1 C31.3 34.6 32.7 34.6 33.8 35.1 L54.6 44.3 C56.1 45 56.1 47 54.6 47.7 L33.8 56.9 C32.7 57.4 31.3 57.4 30.2 56.9 L9.4 47.7 C7.9 47 7.9 45 9.4 44.3 Z"
      :fill="`url(#${baseId})`"
    />

    <!-- Middle Layer: Encrypted Wire / Tunnel Conduit -->
    <path
      d="M30.4 23.6 C31.4 23.1 32.6 23.1 33.6 23.6 L52.2 31.8 C53.5 32.4 53.5 34.2 52.2 34.8 L33.6 43 C32.6 43.5 31.4 43.5 30.4 43 L11.8 34.8 C10.5 34.2 10.5 32.4 11.8 31.8 Z"
      :stroke="`url(#${wireId})`"
      stroke-width="3.8"
      stroke-linejoin="round"
    />

    <!-- Top Layer: Operator Control Deck -->
    <path
      d="M30.2 7.5 C31.3 7 32.7 7 33.8 7.5 L54.6 16.7 C56.1 17.4 56.1 19.4 54.6 20.1 L33.8 29.3 C32.7 29.8 31.3 29.8 30.2 29.3 L9.4 20.1 C7.9 19.4 7.9 17.4 9.4 16.7 Z"
      :fill="`url(#${topId})`"
    />

    <!-- Control Deck Console Accents -->
    <path
      d="M31.1 13.4 C31.7 13.1 32.3 13.1 32.9 13.4 L41.4 17.2 C42.1 17.5 42.1 18.5 41.4 18.8 L32.9 22.6 C32.3 22.9 31.7 22.9 31.1 22.6 L22.6 18.8 C21.9 18.5 21.9 17.5 22.6 17.2 Z"
      fill="#ffffff"
      fill-opacity="0.92"
    />
    <circle cx="32" cy="18" r="2.2" :fill="palette.coreDot" />
  </svg>
</template>
