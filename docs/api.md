# API Documentation

## Overview

The API is built with FastAPI and follows a modular structure.

## Folder Structure

```
api/
├── app/
│   ├── main.py          # Application entry point
│   ├── config.py        # Settings (pydantic-settings)
│   ├── database.py      # Database connection
│   ├── dependencies.py  # Dependency injection
│   ├── logging.py       # Logging setup
│   └── routers/         # API endpoints
```

## Future Directories

As the application grows:

- **`models/`** - SQLModel models for database tables
- **`schemas/`** - Pydantic schemas for request/response validation
- **`services/`** - Business logic layer
