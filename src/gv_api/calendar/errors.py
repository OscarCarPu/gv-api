class NotFoundError(Exception):
    pass


class InvalidError(Exception):
    def __init__(self, detail: str) -> None:
        super().__init__(f"invalid request: {detail}")
