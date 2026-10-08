<template>
  <router-view />
</template>

<script setup>
import { onMounted } from 'vue';

onMounted(() => {
  document.documentElement.classList.remove('dark');
});
</script>
