from functools import lru_cache

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    stage: str = "prod"
    database_url: str

    @property
    def is_dev(self) -> bool:
        return self.stage == "dev"


@lru_cache
def get_settings() -> Settings:
    return Settings()  # type: ignore[call-arg]
