<template>
  <li class="tree-node">
    <div v-if="node.type === 'dir'" class="row dir" @click="open = !open">
      <span class="toggle">{{ open ? '▾' : '▸' }}</span>
      <span class="name">{{ node.name }}</span>
    </div>
    <div v-else class="row file">
      <span class="toggle spacer"></span>
      <span class="name">{{ node.name }}</span>
      <span class="size">{{ formatBytes(node.size) }}</span>
      <button class="btn" @click="$emit('download', node.path)">Download</button>
    </div>
    <ul v-if="node.type === 'dir' && open" class="children">
      <FileTreeNode
        v-for="child in node.children"
        :key="child.path"
        :node="child"
        @download="$emit('download', $event)"
      />
    </ul>
  </li>
</template>

<script setup>
import { ref } from 'vue';
import { formatBytes } from '../utils/format';

defineProps({
  node: { type: Object, required: true },
});
defineEmits(['download']);

const open = ref(false);
</script>

<style scoped>
.tree-node {
  list-style: none;
}
.row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border-radius: 6px;
}
.row.dir {
  cursor: pointer;
  font-weight: 600;
}
.row.dir:hover {
  background-color: var(--color-bg);
}
.toggle {
  width: 1rem;
  color: var(--color-text-muted);
}
.spacer {
  display: inline-block;
}
.name {
  flex: 1;
  word-break: break-all;
}
.size {
  color: var(--color-text-muted);
  font-size: 0.8rem;
}
.children {
  padding-left: 1.25rem;
  margin: 0;
}
</style>
