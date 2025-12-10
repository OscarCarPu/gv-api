import secrets
from typing import Annotated

from fastapi import Depends, HTTPException, Security
from fastapi.security import APIKeyHeader

from app.core.config import get_settings

api_key_header = APIKeyHeader(name="X-API-Key")


async def verify_api_key(api_key: str = Security(api_key_header)) -> None:
    settings = get_settings()
    if not secrets.compare_digest(api_key, settings.api_key):
        raise HTTPException(status_code=403, detail="Invalid API key")


ApiKeyDep = Annotated[None, Depends(verify_api_key)]
