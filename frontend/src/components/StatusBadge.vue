<template>
  <span :class="['status-badge', status]">{{ status }}</span>
</template>

<script setup>
defineProps({
  status: { type: String, required: true },
});
</script>

<style scoped>
.status-badge {
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 600;
  text-transform: capitalize;
}
.status-badge.pending {
  background-color: #fff3cd;
  color: #856404;
}
.status-badge.running {
  background-color: #cce5ff;
  color: #004085;
}
.status-badge.done {
  background-color: #d4edda;
  color: #155724;
}
.status-badge.error {
  background-color: #f8d7da;
  color: #721c24;
}
.status-badge.success {
  background-color: #d4edda;
  color: #155724;
}
.status-badge.failed {
  background-color: #f8d7da;
  color: #721c24;
}
</style>
