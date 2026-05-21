"""PCMI synthetic data generator for distillation and load tests."""

from .generate import generate_records
from .presets import get_preset, list_presets

__all__ = ["generate_records", "get_preset", "list_presets"]
