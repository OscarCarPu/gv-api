from functools import lru_cache
from zoneinfo import ZoneInfo

from pydantic_settings import BaseSettings, SettingsConfigDict

TZ = ZoneInfo("Europe/Madrid")


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env")

    stage: str = "dev"
    postgres_user: str
    postgres_password: str
    postgres_db: str
    postgres_host: str = "localhost"
    postgres_port: int = 54322
    api_key: str
    cors_origins: str = "http://localhost:3000,http://localhost:5173"
    agent_model: str = "ollama:qwen3-fast"
    ollama_base_url: str = "http://127.0.0.1:11434/v1"
    agent_temperature: float = 0
    agent_seed: int = 42
    agent_num_predict: int = 256
    agent_num_ctx: int = 1024

    @property
    def is_dev(self) -> bool:
        return self.stage == "dev"

    @property
    def database_url(self) -> str:
        return f"postgresql+asyncpg://{self.postgres_user}:{self.postgres_password}@{self.postgres_host}:{self.postgres_port}/{self.postgres_db}"

    @property
    def database_url_sync(self) -> str:
        return f"postgresql+psycopg://{self.postgres_user}:{self.postgres_password}@{self.postgres_host}:{self.postgres_port}/{self.postgres_db}"

    @property
    def cors_origins_list(self) -> list[str]:
        return [origin.strip() for origin in self.cors_origins.split(",")]


@lru_cache
def get_settings() -> Settings:
    return Settings()  # type: ignore[call-arg]
