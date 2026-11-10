class NotFoundError(Exception):
    def __init__(self) -> None:
        super().__init__("not found")
