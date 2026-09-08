export function formatBytes(bytes, decimals = 2) {
  if (!bytes || bytes <= 0) return "0 Bytes";
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}

export function formatDate(value) {
  if (!value) return "-";
  // 时间戳口径自适应：数值且小于 1e12 视为秒级（逻辑树 LogicNode.updated_at
  // 是 unix 秒）——毫秒级数值与 ISO 字符串（元数据端点 time.Time 序列化）
  // 直接构造，三者统一到正确时刻
  let v = value;
  if (typeof v === "number" && v > 0 && v < 1e12) v = v * 1000;
  const date = new Date(v);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
}
