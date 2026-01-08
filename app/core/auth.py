from datetime import timedelta

from fastapi import APIRouter, HTTPException, status
from jose import JWTError, jwt

from app.core.config import ErrorMessages, get_settings
from app.core.security import create_access_token, verify_password, verify_totp

settings = get_settings()

router = APIRouter(tags=["auth"])


@router.post("/login")
async def login(password: str):
    if not verify_password(password):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED, detail=ErrorMessages.INVALID_PASSWORD
        )

    temp_token = create_access_token(
        data={"sub": "admin", "mfa": False}, expires_delta=timedelta(minutes=5)
    )

    return {"temp_token": temp_token, "requires_mfa": True}


@router.post("/verify-2fa")
async def verify_2fa(code: str, temp_token: str):
    try:
        payload = jwt.decode(
            temp_token,
            settings.secret_key,
            algorithms=[settings.algorithm],
        )

        if payload.get("mfa") is not False:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED, detail=ErrorMessages.INVALID_TOKEN
            )
    except JWTError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED, detail=ErrorMessages.INVALID_TOKEN
        ) from None
    if not verify_totp(code):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST, detail=ErrorMessages.INVALID_2FA
        )

    final_token = create_access_token(
        data={"sub": "admin", "mfa": True}, expires_delta=timedelta(days=1)
    )
    return {"access_token": final_token, "token_type": "bearer"}
