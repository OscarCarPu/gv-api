import hmac
import warnings
from collections.abc import Callable
from datetime import UTC, datetime, timedelta

import jwt
import pyotp

from gv_api.auth.errors import InvalidCodeError, InvalidPasswordError, InvalidTokenError
from gv_api.config import Config

TMP_TTL = timedelta(minutes=5)
LONG_TTL = timedelta(days=30)

# Any HMAC method is accepted; iat and aud are not checked.
_ALGORITHMS = ["HS256", "HS384", "HS512"]
_OPTIONS: jwt.types.Options = {"verify_iat": False, "verify_aud": False}

# JWT_SECRET has no minimum length requirement.
warnings.filterwarnings("ignore", category=jwt.warnings.InsecureKeyLengthWarning)

TOTPValidator = Callable[[str, str], bool]


def validate_totp(passcode: str, secret: str) -> bool:
    """6 digits, SHA-1, 30 s steps, one step of skew either way."""
    passcode = passcode.strip()
    if len(passcode) != 6:
        return False
    try:
        return pyotp.TOTP(secret.strip().upper()).verify(passcode, valid_window=1)
    except ValueError:  # secret is not base32
        return False


def _same(a: str, b: str) -> bool:
    return hmac.compare_digest(a.encode(), b.encode())


class AuthService:
    def __init__(
        self,
        cfg: Config,
        totp: TOTPValidator = validate_totp,
        now: Callable[[], datetime] = lambda: datetime.now(UTC),
    ) -> None:
        self._cfg = cfg
        self._totp = totp
        self._now = now

    def generate_token(self, ttl: timedelta, kind: str) -> str:
        now = int(self._now().timestamp())
        claims = {"exp": now + int(ttl.total_seconds()), "iat": now, "kind": kind}
        return jwt.encode(claims, self._cfg.jwt_secret, algorithm="HS256")

    def login(self, password: str) -> tuple[str, str]:
        if _same(password, self._cfg.password):
            return self.generate_token(TMP_TTL, "tmp"), "tmp"
        if _same(password, self._cfg.semiprivate_password):
            return self.generate_token(LONG_TTL, "semi"), "semi"
        raise InvalidPasswordError

    def token_kind(self, token: str) -> str | None:
        """The `kind` of a valid, unexpired token, or None."""
        try:
            claims = jwt.decode(
                token, self._cfg.jwt_secret, algorithms=_ALGORITHMS, options=_OPTIONS
            )
        except jwt.InvalidTokenError:
            return None
        kind = claims.get("kind")
        return kind if isinstance(kind, str) else None

    def validate_token(self, token: str, kind: str) -> None:
        if self.token_kind(token) != kind:
            raise InvalidTokenError

    def login_2fa(self, token: str, code: str) -> str:
        self.validate_token(token, "tmp")
        if not self._totp(code, self._cfg.totp_secret):
            raise InvalidCodeError
        return self.generate_token(LONG_TTL, "full")
