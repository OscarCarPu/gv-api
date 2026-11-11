class RunningError(Exception):
    def __init__(self) -> None:
        super().__init__("a backup is already running")


class InvalidNameError(Exception):
    def __init__(self) -> None:
        super().__init__("invalid backup name")


class NotFoundError(Exception):
    def __init__(self) -> None:
        super().__init__("backup not found")
