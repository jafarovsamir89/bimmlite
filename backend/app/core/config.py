from functools import lru_cache

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = "BimmLite"
    env: str = "local"
    api_host: str = "0.0.0.0"
    api_port: int = 8000
    database_url: str = Field(
        default="postgresql+psycopg://bimmlite:bimmlite@localhost:5432/bimmlite"
    )
    allowed_origins: str = "http://localhost:5173"
    bridge_session_token: str = "change-me"
    bridge_heartbeat_seconds: int = 15
    bridge_command_timeout_seconds: int = 10
    log_level: str = "INFO"

    model_config = SettingsConfigDict(
        env_prefix="BIMM_",
        env_file=".env",
        case_sensitive=False,
        env_nested_delimiter="__",
    )

    @property
    def allowed_origins_list(self) -> list[str]:
        return [item.strip() for item in self.allowed_origins.split(",") if item.strip()]


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
