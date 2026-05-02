<template>
  <div class="app-loading" :class="{ 'is-fullscreen': fullscreen }" role="status" aria-live="polite">
    <div class="app-loading__stage">
      <div class="app-loading__orbit app-loading__orbit--outer"></div>
      <div class="app-loading__orbit app-loading__orbit--mid"></div>
      <div class="app-loading__orbit app-loading__orbit--inner"></div>

      <svg class="app-loading__arc" viewBox="0 0 80 80" aria-hidden="true">
        <defs>
          <linearGradient id="appLoadingArc" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stop-color="var(--app-brand)" stop-opacity="0" />
            <stop offset="60%" stop-color="var(--app-brand)" stop-opacity="0.7" />
            <stop offset="100%" stop-color="var(--app-brand-strong, var(--app-brand))" />
          </linearGradient>
        </defs>
        <circle cx="40" cy="40" r="32" fill="none" stroke="url(#appLoadingArc)" stroke-width="3" stroke-linecap="round"
          stroke-dasharray="120 80" />
      </svg>

      <div class="app-loading__core">
        <span class="app-loading__pulse"></span>
        <span class="app-loading__pulse app-loading__pulse--delay"></span>
      </div>
    </div>

    <div class="app-loading__caption">
      <span class="app-loading__title">{{ title }}</span>
      <span class="app-loading__dots" aria-hidden="true">
        <i></i><i></i><i></i>
      </span>
    </div>
    <div v-if="subtitle" class="app-loading__subtitle">{{ subtitle }}</div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    subtitle?: string
    fullscreen?: boolean
  }>(),
  {
    title: "加载中",
    subtitle: "",
    fullscreen: false,
  },
)
</script>

<style scoped>
.app-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 18px;
  padding: 32px;
  color: var(--app-text-muted, #6E6E6E);
}

.app-loading.is-fullscreen {
  min-height: 100vh;
  background:
    radial-gradient(1200px 600px at 20% -10%, var(--app-brand-soft-bg, rgba(99, 91, 255, 0.08)), transparent 60%),
    radial-gradient(900px 500px at 110% 110%, var(--app-brand-soft-bg, rgba(99, 91, 255, 0.08)), transparent 55%),
    var(--app-bg-page, #FAFAFA);
}

.app-loading__stage {
  position: relative;
  width: 96px;
  height: 96px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.app-loading__orbit {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  border: 1px solid var(--app-brand-soft-border, rgba(99, 91, 255, 0.2));
  opacity: 0.55;
  animation: appLoadingPulse 2.4s ease-in-out infinite;
}

.app-loading__orbit--mid {
  inset: 12px;
  border-color: var(--app-brand-ring, rgba(99, 91, 255, 0.18));
  animation-delay: 0.4s;
}

.app-loading__orbit--inner {
  inset: 24px;
  border-color: var(--app-brand-soft-bg, rgba(99, 91, 255, 0.18));
  animation-delay: 0.8s;
}

.app-loading__arc {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  animation: appLoadingSpin 1.4s linear infinite;
  filter: drop-shadow(0 4px 14px var(--app-brand-soft-bg, rgba(99, 91, 255, 0.18)));
}

.app-loading__core {
  position: relative;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--app-brand-gradient, linear-gradient(135deg, var(--app-brand, #635BFF), var(--app-brand-strong, #4D44E0)));
  box-shadow: var(--app-brand-shadow-md, 0 6px 18px rgba(99, 91, 255, 0.35));
}

.app-loading__pulse {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: var(--app-brand, #635BFF);
  opacity: 0.35;
  animation: appLoadingCore 1.8s ease-out infinite;
}

.app-loading__pulse--delay {
  animation-delay: 0.9s;
}

.app-loading__caption {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 14px;
  font-weight: 500;
  letter-spacing: 0.02em;
  color: var(--app-text-muted, #6E6E6E);
}

.app-loading__title {
  background: var(--app-brand-gradient, linear-gradient(135deg, var(--app-brand, #635BFF), var(--app-brand-strong, #4D44E0)));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  font-weight: 600;
}

.app-loading__dots {
  display: inline-flex;
  gap: 3px;
}

.app-loading__dots i {
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--app-brand, #635BFF);
  opacity: 0.4;
  animation: appLoadingDot 1.2s ease-in-out infinite;
}

.app-loading__dots i:nth-child(2) {
  animation-delay: 0.15s;
}

.app-loading__dots i:nth-child(3) {
  animation-delay: 0.3s;
}

.app-loading__subtitle {
  font-size: 12px;
  color: var(--app-text-muted, #A3A3A3);
  opacity: 0.85;
}

@keyframes appLoadingSpin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes appLoadingPulse {
  0%, 100% {
    transform: scale(1);
    opacity: 0.45;
  }
  50% {
    transform: scale(1.08);
    opacity: 0.85;
  }
}

@keyframes appLoadingCore {
  0% {
    transform: scale(0.6);
    opacity: 0.55;
  }
  100% {
    transform: scale(2.4);
    opacity: 0;
  }
}

@keyframes appLoadingDot {
  0%, 80%, 100% {
    opacity: 0.3;
    transform: translateY(0);
  }
  40% {
    opacity: 1;
    transform: translateY(-3px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .app-loading__orbit,
  .app-loading__arc,
  .app-loading__pulse,
  .app-loading__dots i {
    animation: none;
  }
}
</style>
