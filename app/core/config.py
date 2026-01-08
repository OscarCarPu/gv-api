from functools import lru_cache
from zoneinfo import ZoneInfo

from pydantic_settings import BaseSettings, SettingsConfigDict

TZ = ZoneInfo("Europe/Madrid")


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore", env_prefix="")

    # --- APP ---
    stage: str = "dev"

    # --- DATABASE ---
    postgres_user: str
    postgres_password: str
    postgres_db: str
    postgres_host: str = "localhost"
    postgres_port: int = 54322

    # --- LLM CONNECTIVITY ---
    groq_api_key: str
    agent_model: str = "groq:llama-3.3-70b-versatile"
    agent_temperature: float = 0

    # --- SECURITY ---
    secret_key: str
    totp_secret: str
    hashed_password: str
    algorithm: str = "HS256"
    cors_origins: str = "http://localhost:3000,http://localhost:5173"

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


# --- CONSTANTS ---


class ErrorMessages:
    INVALID_PASSWORD = "Contraseña incorrecta"
    INVALID_2FA = "Código de autenticación inválido"
    INVALID_TOKEN = "Token inválido"
