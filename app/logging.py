import logging

from app.config import get_settings


def setup_logging() -> None:
    settings = get_settings()

    logging.basicConfig(
        level=logging.DEBUG if settings.is_dev else logging.WARNING,
        format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    )


def get_logger(name: str) -> logging.Logger:
    return logging.getLogger(name)
