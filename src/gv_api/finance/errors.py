class NotFoundError(Exception):
    pass


class AccountInUseError(Exception):
    pass


class CategoryInUseError(Exception):
    pass


class InvalidInputError(Exception):
    """A reference (account, category, parent) that does not exist or is not allowed."""


class CategoryMismatchError(Exception):
    pass


class BudgetTransferError(Exception):
    pass
