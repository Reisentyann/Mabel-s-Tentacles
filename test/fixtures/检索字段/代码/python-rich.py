# SPDX-License-Identifier: MIT
"""Feature-rich Python sample for code retrieval.

The unique retrieval marker is SEARCH_PY_FEATURE_2026.
"""

import json
import os
from collections import Counter

SEARCH_MARKER = "SEARCH_PY_FEATURE_2026"


# TODO: support callers that provide an iterable instead of a list.
def normalize_counts(values):
    """Return lowercase non-empty values and their frequencies."""
    counts = Counter(value.strip().lower() for value in values if value.strip())
    return dict(counts)


class FeatureReport:
    """Collect labelled values and serialize a small report."""

    def __init__(self, title):
        self.title = title
        self.items = []

    def add_item(self, label, value):
        # FIXME: reject duplicate labels instead of retaining both values.
        self.items.append({"label": label, "value": value})

    def to_json(self):
        return json.dumps(
            {"marker": SEARCH_MARKER, "title": self.title, "items": self.items},
            sort_keys=True,
        )


def environment_label(name):
    """Read an optional process environment label."""
    return os.environ.get(name, "")
