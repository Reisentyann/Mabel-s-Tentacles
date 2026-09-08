#!/bin/bash
# migrate-legacy.sh — 旧版存量迁移（2026-09-08 部署批次）：
# 旧物理布局（data/<明文路径>）+ 无前缀键 → 新世界（uuid 派生位 + ~agent/ 键前缀）
# 幂等可重复跑（已迁移行 NOT LIKE '~%' 被排除）；旧目录不动（保留原件）
set -e
PG() { docker exec agent_postgres psql -U postgres -d agent_db "$@"; }

rows=$(PG -t -A -F$'\t' -c "SELECT file_path, uuid FROM file_metadata WHERE file_path NOT LIKE '~%' AND is_deleted = false")
echo "$rows" | while IFS=$'\t' read -r p u; do
  [ -z "$p" ] && continue
  src="/opt/Mabel-s-Tentacles/data/$p"
  ext="${p##*.}"
  dst="/opt/mabel/data/${u:0:2}/$u.$ext"
  if [ -f "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src" "$dst"
    docker exec -i agent_postgres psql -U postgres -d agent_db -q -v p="$p" -v u="$u" <<SQL
UPDATE file_metadata SET file_path = '~agent/' || :'p', moved_from = :'p', missing_rounds = 0 WHERE uuid = :'u'
SQL
    echo "MIGRATED: $p -> ~agent/$p"
  else
    echo "SKIP(旧文件缺失): $p"
  fi
done

echo "=== 迁移后全表 ==="
PG -t -A -c "SELECT file_path || ' | ' || moved_from FROM file_metadata ORDER BY file_path"
