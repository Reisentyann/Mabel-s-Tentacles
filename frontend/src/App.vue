<template>
  <router-view />
</template>

<script setup>
// 暗色书房主题为唯一主题（2026-09-08）：html.dark 挂 Element Plus 暗色变量
import { onMounted } from 'vue';

onMounted(() => {
  document.documentElement.classList.add('dark');
});
</script>
