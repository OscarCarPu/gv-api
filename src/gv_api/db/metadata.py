from sqlalchemy import MetaData

# One MetaData for every domain's tables.py; Alembic and the drift test read it.
metadata = MetaData()
