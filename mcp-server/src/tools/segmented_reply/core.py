import asyncio
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

from src.tools.segmented_reply.config import CONFIG_PATH, load_segmented_reply_config


BASE_DIR = Path("data").resolve()
MAX_CONTENT_SIZE = 5 * 1024 * 1024
MIN_SEGMENT_LENGTH = 50
MAX_SEGMENT_LENGTH = 5000
MAX_INTERVAL_SECONDS = 10.0
QUEUE_DIR_NAME = ".queues"


def _safe_session_id(session_id: str) -> str:
    safe_id = re.sub(r"[^0-9A-Za-z_-]", "_", session_id.strip())
    return safe_id[:64] or "default"


def _safe_output_dir(output_dir: str) -> Path:
    clean_dir = str(output_dir or "segmented_replies").strip().lstrip("/\\")
    if not clean_dir or ":" in clean_dir:
        clean_dir = "segmented_replies"

    reply_dir = (BASE_DIR / clean_dir).resolve()
    if not reply_dir.is_relative_to(BASE_DIR):
        return (BASE_DIR / "segmented_replies").resolve()
    return reply_dir


def _queue_dir(output_dir: str) -> Path:
    queue_dir = (_safe_output_dir(output_dir) / QUEUE_DIR_NAME).resolve()
    queue_dir.mkdir(parents=True, exist_ok=True)
    return queue_dir


def _queue_path(session_id: str, output_dir: str) -> Path:
    return (_queue_dir(output_dir) / f"{_safe_session_id(session_id)}.json").resolve()


def _save_reply_queue(session_id: str, output_dir: str, queue: Dict[str, Any]) -> Path:
    path = _queue_path(session_id, output_dir)
    if not path.is_relative_to(_queue_dir(output_dir)):
        raise ValueError("Invalid segmented reply queue path.")
    path.write_text(json.dumps(queue, ensure_ascii=False, indent=2), encoding="utf-8")
    return path


def _load_reply_queue(session_id: str, output_dir: str) -> Optional[Dict[str, Any]]:
    path = _queue_path(session_id, output_dir)
    if not path.exists():
        return None

    with path.open("r", encoding="utf-8") as file:
        queue = json.load(file)

    if not isinstance(queue, dict):
        return None
    return queue


def _delete_reply_queue(session_id: str, output_dir: str) -> None:
    path = _queue_path(session_id, output_dir)
    if path.exists():
        path.unlink()


def _apply_content_filter(content: str, content_filter: Dict[str, Any]) -> Tuple[bool, str, List[str]]:
    blocked_words = [str(word) for word in content_filter.get("blocked_words", []) if str(word)]
    matched_blocked_words = [word for word in blocked_words if word in content]
    if matched_blocked_words:
        return False, content, matched_blocked_words

    filtered_content = content
    replace_rules = content_filter.get("replace_rules", {})
    if isinstance(replace_rules, dict):
        for source, target in replace_rules.items():
            filtered_content = filtered_content.replace(str(source), str(target))

    return True, filtered_content, []


def _split_by_markers(content: str, split_words: List[str]) -> List[str]:
    markers = [marker for marker in split_words if marker]
    if not markers:
        return [content.strip()]

    pattern = "|".join(re.escape(marker) for marker in markers)
    return [segment.strip() for segment in re.split(pattern, content) if segment.strip()]


def _split_by_length(text: str, max_length: int) -> List[str]:
    segments = []
    remaining = text.strip()

    while remaining:
        if len(remaining) <= max_length:
            segments.append(remaining)
            break

        split_index = max(
            remaining.rfind("\n", 0, max_length + 1),
            remaining.rfind("。", 0, max_length + 1),
            remaining.rfind("！", 0, max_length + 1),
            remaining.rfind("？", 0, max_length + 1),
            remaining.rfind("；", 0, max_length + 1),
            remaining.rfind("，", 0, max_length + 1),
            remaining.rfind(".", 0, max_length + 1),
            remaining.rfind("!", 0, max_length + 1),
            remaining.rfind("?", 0, max_length + 1),
            remaining.rfind(" ", 0, max_length + 1),
        )

        if split_index <= 0:
            split_index = max_length
        else:
            split_index += 1

        segments.append(remaining[:split_index].strip())
        remaining = remaining[split_index:].strip()

    return [segment for segment in segments if segment]


def _build_segments(content: str, split_words: List[str], segment_length_threshold: int) -> List[str]:
    marker_segments = _split_by_markers(content, split_words)
    segments = []

    for segment in marker_segments:
        if len(segment) <= segment_length_threshold:
            segments.append(segment)
        else:
            segments.extend(_split_by_length(segment, segment_length_threshold))

    return segments


async def segmented_reply_tool(content: str, session_id: str = "default") -> Dict[str, Any]:
    """Create segmented reply files and initialize a queued multi-reply session."""
    try:
        if not content or not content.strip():
            return {"success": False, "message": "Error: content cannot be empty."}

        if len(content.encode("utf-8")) > MAX_CONTENT_SIZE:
            return {"success": False, "message": "Security Error: Content exceeds 5MB limit."}

        config = load_segmented_reply_config()
        allowed, filtered_content, blocked_words = _apply_content_filter(content, config.get("content_filter", {}))
        if not allowed:
            return {
                "success": False,
                "message": "Content blocked by segmented reply content_filter.blocked_words.",
                "blocked_words": blocked_words,
                "config_path": str(CONFIG_PATH),
            }

        segment_length_threshold = int(config.get("segment_length_threshold", 500))
        if segment_length_threshold < MIN_SEGMENT_LENGTH or segment_length_threshold > MAX_SEGMENT_LENGTH:
            return {
                "success": False,
                "message": f"Error: segment_length_threshold must be between {MIN_SEGMENT_LENGTH} and {MAX_SEGMENT_LENGTH}.",
                "config_path": str(CONFIG_PATH),
            }

        max_segments = int(config.get("max_segments", 20))
        interval_seconds = float(config.get("interval_seconds", 1.0))
        interval_seconds = max(0.0, min(interval_seconds, MAX_INTERVAL_SECONDS))
        split_words = [str(word) for word in config.get("split_words", [])]
        force_segmented_reply = bool(config.get("force_segmented_reply", True))

        if force_segmented_reply or len(filtered_content) > segment_length_threshold:
            segments = _build_segments(filtered_content, split_words, segment_length_threshold)
        else:
            segments = [filtered_content.strip()]

        if not segments:
            return {"success": False, "message": "Error: no valid reply segments were generated."}

        if len(segments) > max_segments:
            return {"success": False, "message": f"Error: too many segments ({len(segments)}). Maximum is {max_segments}."}

        reply_dir = _safe_output_dir(config.get("output_dir", "segmented_replies"))
        reply_dir.mkdir(parents=True, exist_ok=True)

        safe_session_id = _safe_session_id(session_id)
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
        files = []

        for index, segment in enumerate(segments, start=1):
            file_name = f"{timestamp}_{safe_session_id}_{index:02d}.txt"
            target_path = (reply_dir / file_name).resolve()

            if not target_path.is_relative_to(reply_dir):
                return {"success": False, "message": "Security Error: Invalid segmented reply path."}

            target_path.write_text(segment, encoding="utf-8")
            files.append(target_path.relative_to(BASE_DIR).as_posix())

        first_reply = segments[0]
        remaining_segments = segments[1:]
        queue_path = None
        if remaining_segments:
            queue_path = _save_reply_queue(
                session_id,
                config.get("output_dir", "segmented_replies"),
                {
                    "created_at": datetime.now().isoformat(),
                    "session_id": safe_session_id,
                    "interval_seconds": interval_seconds,
                    "segments": remaining_segments,
                    "next_index": 2,
                    "total": len(segments),
                },
            )
        else:
            _delete_reply_queue(session_id, config.get("output_dir", "segmented_replies"))

        return {
            "success": True,
            "message": first_reply,
            "reply": first_reply,
            "segment_count": len(files),
            "remaining_count": len(remaining_segments),
            "has_more": bool(remaining_segments),
            "next_tool": "next_reply" if remaining_segments else None,
            "interval_seconds": interval_seconds,
            "force_segmented_reply": force_segmented_reply,
            "config_path": str(CONFIG_PATH),
            "queue_path": queue_path.relative_to(BASE_DIR).as_posix() if queue_path else None,
            "files": files,
            "segments": segments,
        }

    except Exception as e:
        return {"success": False, "message": f"Segmented reply failed: {str(e)}"}


async def next_reply_tool(session_id: str = "default") -> Dict[str, Any]:
    """Return the next queued segmented reply to trigger another visible bot reply."""
    try:
        config = load_segmented_reply_config()
        output_dir = config.get("output_dir", "segmented_replies")
        queue = _load_reply_queue(session_id, output_dir)

        if not queue:
            return {
                "success": False,
                "message": "No pending segmented reply for this session.",
                "reply": None,
                "has_more": False,
            }

        segments = queue.get("segments", [])
        if not isinstance(segments, list) or not segments:
            _delete_reply_queue(session_id, output_dir)
            return {
                "success": False,
                "message": "No pending segmented reply for this session.",
                "reply": None,
                "has_more": False,
            }

        interval_seconds = float(queue.get("interval_seconds", 0.0))
        interval_seconds = max(0.0, min(interval_seconds, MAX_INTERVAL_SECONDS))
        if interval_seconds > 0:
            await asyncio.sleep(interval_seconds)

        reply = str(segments.pop(0))
        next_index = int(queue.get("next_index", 2))
        total = int(queue.get("total", next_index))
        has_more = bool(segments)

        if has_more:
            queue["segments"] = segments
            queue["next_index"] = next_index + 1
            _save_reply_queue(session_id, output_dir, queue)
        else:
            _delete_reply_queue(session_id, output_dir)

        return {
            "success": True,
            "message": reply,
            "reply": reply,
            "segment_index": next_index,
            "segment_count": total,
            "remaining_count": len(segments),
            "has_more": has_more,
            "next_tool": "next_reply" if has_more else None,
        }

    except Exception as e:
        return {"success": False, "message": f"Next reply failed: {str(e)}", "reply": None, "has_more": False}