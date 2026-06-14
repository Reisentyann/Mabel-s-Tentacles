import json
from pathlib import Path
from typing import Any, Dict


CONFIG_PATH = Path(__file__).with_name("config.json")

DEFAULT_CONFIG: Dict[str, Any] = {
    "force_segmented_reply": True,
    "interval_seconds": 1.0,
    "segment_length_threshold": 500,
    "max_segments": 50,
    "output_dir": "segmented_replies",
    "split_words": [
    "---",
    "\n---\n",
    ",",
    "，",
    "……",
    "?",
    "？",
    "!",
    "！",
    "。"
  ],
    "content_filter": {
        "blocked_words": [],
        "replace_rules": {},
    },
}


def load_segmented_reply_config() -> Dict[str, Any]:
    if not CONFIG_PATH.exists():
        return DEFAULT_CONFIG.copy()

    with CONFIG_PATH.open("r", encoding="utf-8") as file:
        user_config = json.load(file)

    config = DEFAULT_CONFIG.copy()
    config.update(user_config)

    content_filter = DEFAULT_CONFIG["content_filter"].copy()
    content_filter.update(user_config.get("content_filter", {}))
    config["content_filter"] = content_filter

    return config