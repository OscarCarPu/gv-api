"""Common validation and sanitization functions."""

import html

from app.common.enums import Icons

# Field length constants
NAME_MAX_LENGTH = 25
UNIT_MAX_LENGTH = 10
COLOR_MAX_LENGTH = 7
DESCRIPTION_MAX_LENGTH = 500


def sanitize_name(value: str) -> str:
    """Strip whitespace and validate name is not empty."""
    value = html.escape(value.strip())
    if not value:
        raise ValueError("Name cannot be empty")
    if len(value) > NAME_MAX_LENGTH:
        raise ValueError(f"Name cannot exceed {NAME_MAX_LENGTH} characters")
    return value


def sanitize_optional_string(value: str | None) -> str | None:
    """Strip whitespace from optional string, return None if empty."""
    if value:
        return value.strip() or None
    return value


def sanitize_description(value: str | None) -> str | None:
    """Strip whitespace from description, return None if empty."""
    result = sanitize_optional_string(value)
    if result and len(result) > DESCRIPTION_MAX_LENGTH:
        raise ValueError(f"Description cannot exceed {DESCRIPTION_MAX_LENGTH} characters")
    return result


def sanitize_unit(value: str | None) -> str | None:
    """Strip whitespace from unit, return None if empty."""
    result = sanitize_optional_string(value)
    if result and len(result) > UNIT_MAX_LENGTH:
        raise ValueError(f"Unit cannot exceed {UNIT_MAX_LENGTH} characters")
    return result


def sanitize_color(value: str) -> str:
    """Strip, uppercase, and validate hex color format."""
    value = value.strip().upper()
    valid_chars = "0123456789ABCDEF"
    if (
        len(value) != COLOR_MAX_LENGTH
        or not value.startswith("#")
        or not all(c in valid_chars for c in value[1:])
    ):
        raise ValueError("Invalid color format (must be #RRGGBB)")
    return value


def validate_icon(value: str) -> str:
    """Strip and validate icon against allowed values."""
    value = value.strip()
    valid_icons = {icon.value for icon in Icons}
    if value not in valid_icons:
        raise ValueError("Invalid icon")
    return value
