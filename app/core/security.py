from datetime import datetime, timedelta
from typing import Annotated

import pyotp
from fastapi import Depends, HTTPException, status
from fastapi.security import OAuth2PasswordBearer
from jose import JWTError, jwt
from pwdlib import PasswordHash
from pwdlib.hashers.argon2 import Argon2Hasher

from app.core.config import TZ, get_settings

settings = get_settings()
pwd_hash = PasswordHash((Argon2Hasher(),))
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="/api/v1/auth/login")


def create_access_token(data: dict, expires_delta: timedelta | None = None):
    to_encode = data.copy()
    expire = datetime.now(TZ) + (expires_delta or timedelta(minutes=15))
    to_encode.update({"exp": expire})
    return jwt.encode(to_encode, settings.secret_key, algorithm=settings.algorithm)


def verify_password(plain_password: str) -> bool:
    return pwd_hash.verify(plain_password, settings.hashed_password)


def verify_totp(code: str) -> bool:
    totp = pyotp.TOTP(settings.totp_secret)
    return totp.verify(code)


# --- DEPENDENCIES ---
async def require_auth(token: Annotated[str, Depends(oauth2_scheme)]):
    credentials_exception = HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Could not validate credentials",
        headers={"WWW-Authenticate": "Bearer"},
    )
    try:
        payload = jwt.decode(token, settings.secret_key, algorithms=[settings.algorithm])
        username: str | None = payload.get("sub")
        mfa_status: bool = payload.get("mfa", False)
        if username is None or not mfa_status:
            raise credentials_exception
        return username
    except JWTError:
        raise credentials_exception from None


RequireAuth = Annotated[str, Depends(require_auth)]
