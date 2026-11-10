class AuthError(Exception):
    message = ""

    def __init__(self) -> None:
        super().__init__(self.message)


class InvalidPasswordError(AuthError):
    message = "invalid password"


class InvalidTokenError(AuthError):
    message = "invalid token"


class InvalidCodeError(AuthError):
    message = "invalid 2fa code"
