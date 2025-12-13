# Gestor de Vida API - Comprehensive Improvement Analysis

This document provides a thorough analysis of potential improvements, architectural decisions, and recommendations for the Gestor de Vida API application.

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Security Improvements](#2-security-improvements)
3. [Architectural Improvements](#3-architectural-improvements)
4. [Performance Optimizations](#4-performance-optimizations)
5. [Code Quality Improvements](#5-code-quality-improvements)
6. [Testing Improvements](#6-testing-improvements)
7. [DevOps & Infrastructure](#7-devops--infrastructure)
8. [Business Logic Improvements](#8-business-logic-improvements)
9. [API Design Improvements](#9-api-design-improvements)
10. [Database Improvements](#10-database-improvements)
11. [Documentation Improvements](#11-documentation-improvements)
12. [Future Architecture Considerations](#12-future-architecture-considerations)

---

## 1. Executive Summary

### Current State
The application is a well-structured FastAPI-based habit tracking API with:
- Clean layered architecture (Router -> Service -> Repository -> Model)
- Async-first design with SQLAlchemy 2.0
- Good validation at multiple layers
- Docker containerization support

### Key Strengths
- Modern Python 3.14 with type hints throughout
- Generic repository pattern enables easy domain expansion
- Comprehensive validation (Pydantic + SQLAlchemy validators + service layer)
- Good separation of concerns

### Priority Improvements (High Impact)
1. **Security**: JWT/OAuth2 authentication, rate limiting, secrets management
2. **Testing**: Expand test coverage, add integration tests for habits
3. **Observability**: Structured logging, metrics, tracing
4. **Database**: Connection pooling configuration, soft deletes
5. **Error Handling**: Global exception handler, structured error responses

---

## 2. Security Improvements

### 2.1 Authentication Enhancement

**Current State**: Single API key authentication via `X-API-Key` header.

**Issues**:
- Single shared API key for all clients
- No user identity tracking
- No token expiration/refresh mechanism
- API key in environment variable without rotation support

**Recommendations**:

#### Option A: JWT-based Authentication (Recommended for multi-user)
```python
# Example structure
from fastapi_jwt_auth import AuthJWT

class AuthSettings(BaseModel):
    authjwt_secret_key: str
    authjwt_access_token_expires: timedelta = timedelta(minutes=15)
    authjwt_refresh_token_expires: timedelta = timedelta(days=7)
```

Benefits:
- User-specific authentication
- Token expiration and refresh
- Stateless authentication
- Easy integration with frontend apps

#### Option B: OAuth2 with external provider (Recommended for personal use)
For a personal life management app, consider:
- **Authelia** or **Authentik** for self-hosted SSO
- **Auth0** or **Clerk** for managed solutions
- Keeps user management out of your application

### 2.2 API Key Improvements (If Keeping Current Approach)

```python
# config.py improvements
class Settings(BaseSettings):
    api_key: str
    api_key_rotation_date: datetime | None = None  # Track key age

    @field_validator("api_key")
    @classmethod
    def validate_api_key_strength(cls, v: str) -> str:
        if len(v) < 32:
            raise ValueError("API key must be at least 32 characters")
        return v
```

### 2.3 Rate Limiting

**Current State**: No rate limiting implemented.

**Recommendation**: Add `slowapi` for rate limiting:

```python
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.util import get_remote_address

limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter

@router.post("")
@limiter.limit("10/minute")
async def create_habit(...):
    ...
```

### 2.4 Security Headers

**Current State**: No security headers configured.

**Recommendation**: Add security middleware:

```python
from secure import Secure

secure_headers = Secure()

@app.middleware("http")
async def add_security_headers(request: Request, call_next):
    response = await call_next(request)
    secure_headers.framework.fastapi(response)
    return response
```

### 2.5 Input Sanitization Gaps

**Current Issues**:
- `app/common/validations.py:12-19`: Name sanitization strips whitespace but doesn't prevent XSS characters
- Description field allows any content without HTML escaping

**Recommendation**:
```python
import html

def sanitize_name(value: str) -> str:
    value = html.escape(value.strip())
    # ... rest of validation
```

### 2.6 Database Password in Connection String

**Current State**: Password directly interpolated in connection URL (`config.py:27`).

**Recommendation**: Use URL encoding for special characters:
```python
from urllib.parse import quote_plus

@property
def database_url(self) -> str:
    password = quote_plus(self.postgres_password)
    return f"postgresql+asyncpg://{self.postgres_user}:{password}@..."
```

### 2.7 Secrets Management

**Current State**: Secrets in `.env` file.

**Recommendations**:
- **Development**: Continue with `.env` but use stronger example values
- **Production**: Use secrets manager (HashiCorp Vault, AWS Secrets Manager, Docker secrets)
- Add `.env` to pre-commit checks to prevent committing

---

## 3. Architectural Improvements

### 3.1 Dependency Injection Container

**Current State**: Manual dependency injection in routers.

**Recommendation**: Consider `dependency-injector` for complex scenarios:

```python
from dependency_injector import containers, providers

class Container(containers.DeclarativeContainer):
    config = providers.Configuration()

    habit_repository = providers.Factory(
        HabitRepository,
        session=providers.Dependency()
    )

    habit_service = providers.Factory(
        HabitService,
        habit_repo=habit_repository,
        log_repo=log_repository
    )
```

Benefits:
- Centralized dependency configuration
- Easier testing with mock injection
- Better visibility of dependencies

### 3.2 Event-Driven Architecture

**Current State**: Synchronous request-response only.

**Recommendation**: Add event system for cross-cutting concerns:

```python
# events.py
from typing import Protocol, Callable

class Event(Protocol):
    pass

class HabitCompleted(Event):
    habit_id: int
    log_date: date
    streak: int

class EventBus:
    _handlers: dict[type[Event], list[Callable]] = {}

    @classmethod
    def subscribe(cls, event_type: type[Event], handler: Callable):
        cls._handlers.setdefault(event_type, []).append(handler)

    @classmethod
    async def publish(cls, event: Event):
        for handler in cls._handlers.get(type(event), []):
            await handler(event)
```

Use cases:
- Send notifications when streaks are broken
- Update statistics asynchronously
- Trigger gamification rewards

### 3.3 CQRS Pattern for Analytics

**Current State**: Same models for reads and writes.

**Recommendation**: Separate read models for analytics:

```python
# Read-optimized model for daily dashboard
class DailyDashboard(Base):
    __tablename__ = "daily_dashboard_view"

    date: Mapped[date]
    total_habits: Mapped[int]
    completed_habits: Mapped[int]
    completion_rate: Mapped[Decimal]
    streaks_maintained: Mapped[int]
    # ... materialized view or denormalized table
```

### 3.4 Module Registration Pattern

**Current State**: Routers manually imported and registered in `main.py`.

**Recommendation**: Auto-discovery pattern:

```python
# main.py
import importlib
import pkgutil

def register_routers(app: FastAPI):
    for _, name, _ in pkgutil.iter_modules(["app"]):
        if name not in ["core", "common"]:
            module = importlib.import_module(f"app.{name}")
            if hasattr(module, "router"):
                app.include_router(module.router, prefix="/api/v1")
```

---

## 4. Performance Optimizations

### 4.1 Database Connection Pooling

**Current State**: Default SQLAlchemy pool settings (`database.py:23`).

**Recommendation**: Configure pool explicitly:

```python
engine = create_async_engine(
    settings.database_url,
    echo=settings.is_dev,
    pool_size=5,           # Connections to keep open
    max_overflow=10,       # Additional connections when pool exhausted
    pool_pre_ping=True,    # Verify connections before use
    pool_recycle=3600,     # Recycle connections after 1 hour
)
```

### 4.2 Query Optimization

**Issue in `service.py:120`**: Stats calculation loads all logs then filters in Python.

```python
# Current (inefficient for large datasets)
logs = list(await self.log_repo.get_logs_in_range(habit_id, start_date))
targets_met = sum(1 for log in logs if self.check_target_met(habit, log.value))
```

**Recommendation**: Push aggregation to database:

```python
async def get_stats_aggregated(self, habit_id: int, days: int = 30) -> HabitStats:
    """Database-level statistics calculation."""
    stmt = select(
        func.count(HabitLog.id).label("total_logs"),
        func.avg(HabitLog.value).label("average_value"),
        # Add CASE statements for target_met calculation
    ).where(
        HabitLog.habit_id == habit_id,
        HabitLog.log_date >= start_date
    )
    # ...
```

### 4.3 Streak Calculation Optimization

**Issue in `service.py:162-211`**: Streak calculation iterates day-by-day, inefficient for long date ranges.

**Recommendation**: Use SQL window functions:

```sql
-- Example PostgreSQL query for streak calculation
WITH ranked_logs AS (
    SELECT
        log_date,
        log_date - (ROW_NUMBER() OVER (ORDER BY log_date))::int AS grp
    FROM habit_log
    WHERE habit_id = $1 AND value >= target_value
)
SELECT grp, COUNT(*) as streak_length
FROM ranked_logs
GROUP BY grp
ORDER BY streak_length DESC
LIMIT 1;
```

### 4.4 Response Caching

**Recommendation**: Add caching for expensive endpoints:

```python
from fastapi_cache import FastAPICache
from fastapi_cache.backends.redis import RedisBackend
from fastapi_cache.decorator import cache

@router.get("/{habit_id}/stats")
@cache(expire=300)  # 5 minutes
async def get_habit_stats(...):
    ...
```

Consider caching:
- `/daily` endpoint (cache per date)
- `/{habit_id}/stats` (cache with short TTL)
- `/{habit_id}/streak` (invalidate on log creation)

### 4.5 Pagination Improvements

**Current State**: Offset-based pagination (`schemas.py:28-30`).

**Issue**: Offset pagination degrades with large datasets.

**Recommendation**: Add cursor-based pagination option:

```python
class CursorPaginatedResponse(BaseModel, Generic[T]):
    items: list[T]
    next_cursor: str | None
    has_more: bool

# Usage: GET /habits?cursor=eyJpZCI6MTAwfQ==
```

### 4.6 Bulk Operations

**Current State**: No bulk endpoints.

**Recommendation**: Add bulk log creation for mobile sync:

```python
@router.post("/{habit_id}/logs/bulk")
async def bulk_create_logs(
    habit_id: int,
    logs: list[HabitLogBody],  # Max 100 items
    service: LogServiceDep
) -> list[HabitLogRead]:
    return await service.bulk_upsert(habit_id, logs)
```

---

## 5. Code Quality Improvements

### 5.1 Duplicate Validation Logic

**Issue**: Validation logic duplicated between:
- `app/habits/models.py:93-117` (Habit.validate_target_config)
- `app/habits/validations.py:8-37` (validate_target_config)
- `app/habits/schemas.py:84-93` (HabitBase._validate_target_config)

**Recommendation**: Single source of truth:

```python
# validations.py - THE source
def validate_target_config(...): ...

# models.py - Call shared function
def validate_target_config(self) -> None:
    from app.habits.validations import validate_target_config as validate
    validate(self.value_type, self.comparison_type, ...)

# schemas.py - Call shared function
@model_validator(mode="after")
def _validate_target_config(self) -> Self:
    validate_target_config(self.value_type, ...)
    return self
```

### 5.2 Error Message Consistency

**Issue**: Error messages inconsistent across layers:

```python
# models.py:97
raise ValueError("Boolean habits cannot have numeric targets")

# validations.py:18
raise ValueError("Boolean habits cannot have numeric targets")

# But different style in:
# validations.py:27
raise ValueError("target_min must be less than target_max")
```

**Recommendation**: Centralized error messages:

```python
# constants.py
class ErrorMessages:
    BOOLEAN_NO_NUMERIC_TARGETS = "Boolean habits cannot have numeric targets"
    RANGE_REQUIRES_MIN_MAX = "Range comparison requires target_min and target_max"
    # ...
```

### 5.3 Unused Parameter Warning

**Issue in `router.py:277-278`**:

```python
async def update_log(habit_id: int, log_id: int, data: HabitLogUpdate, service: LogServiceDep):
    return await service.update(log_id, data)  # habit_id unused
```

**Recommendation**: Either validate habit_id ownership or remove from signature:

```python
async def update_log(habit_id: int, log_id: int, ...):
    log = await service.get(log_id)
    if log.habit_id != habit_id:
        raise NotFoundError("Log not found for this habit")
    return await service.update(log_id, data)
```

### 5.4 Type Hints Improvements

**Issue in `repository.py:43`**: `order_by` parameter type is complex.

**Recommendation**: Use TypeAlias for clarity:

```python
from sqlalchemy import ColumnElement, UnaryExpression
from typing import TypeAlias

OrderByClause: TypeAlias = UnaryExpression[Any] | ColumnElement[Any] | None
```

### 5.5 Global Engine Pattern

**Issue in `database.py:17`**: Global mutable state.

```python
engine: AsyncEngine | None = None
```

**Recommendation**: Use application state or context:

```python
# Option 1: App state (FastAPI pattern)
app.state.engine = engine

# Option 2: ContextVar for request-scoped
from contextvars import ContextVar
_engine_ctx: ContextVar[AsyncEngine] = ContextVar("engine")
```

### 5.6 Hardcoded Timezone

**Issue in `config.py:6`**:

```python
TZ = ZoneInfo("Europe/Madrid")
```

**Recommendation**: Make configurable:

```python
class Settings(BaseSettings):
    timezone: str = "Europe/Madrid"

    @cached_property
    def tz(self) -> ZoneInfo:
        return ZoneInfo(self.timezone)
```

---

## 6. Testing Improvements

### 6.1 Current Coverage Analysis

**Current State**:
- Only 2 test functions in `test_health.py`
- No tests for habits CRUD operations
- No tests for business logic (streaks, stats)
- No tests for validation logic

### 6.2 Recommended Test Structure

```
tests/
├── conftest.py              # Shared fixtures
├── unit/                    # Unit tests (no DB)
│   ├── test_validations.py  # Input validation
│   ├── test_streak_calc.py  # Streak algorithm
│   └── test_schemas.py      # Pydantic validation
├── integration/             # Integration tests (with DB)
│   ├── test_habit_crud.py   # Habit CRUD operations
│   ├── test_log_crud.py     # Log CRUD operations
│   ├── test_habit_stats.py  # Statistics calculations
│   └── test_daily_progress.py
├── e2e/                     # End-to-end tests
│   └── test_habit_flow.py   # Complete user flows
└── fixtures/                # Test data factories
    ├── habits.py
    └── logs.py
```

### 6.3 Missing Test Cases

#### Unit Tests Needed:
```python
# test_validations.py
def test_sanitize_name_strips_whitespace():
    assert sanitize_name("  test  ") == "test"

def test_sanitize_name_rejects_empty():
    with pytest.raises(ValueError, match="cannot be empty"):
        sanitize_name("   ")

def test_sanitize_color_valid_format():
    assert sanitize_color("#ff0000") == "#FF0000"

def test_validate_target_config_boolean_no_numeric():
    with pytest.raises(ValueError):
        validate_target_config(
            ValueType.boolean,
            ComparisonType.greater_than,  # Invalid for boolean
            Decimal("5"),
            None, None
        )
```

#### Integration Tests Needed:
```python
# test_habit_crud.py
async def test_create_habit(client: AsyncClient):
    response = await client.post("/api/v1/habits", json={
        "name": "Exercise",
        "value_type": "boolean",
    })
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "Exercise"
    assert data["id"] is not None

async def test_create_habit_duplicate_name(client: AsyncClient, habit_factory):
    await habit_factory(name="Exercise")
    response = await client.post("/api/v1/habits", json={
        "name": "Exercise",
        "value_type": "boolean",
    })
    assert response.status_code == 409

async def test_get_habit_stats_no_logs(client: AsyncClient, habit_factory):
    habit = await habit_factory()
    response = await client.get(f"/api/v1/habits/{habit.id}/stats")
    assert response.status_code == 200
    data = response.json()
    assert data["total_logs"] == 0
    assert data["completion_rate"] == 0
```

### 6.4 Test Fixtures Improvement

**Current Issue in `conftest.py:63-69`**: Cleanup happens after yield (teardown), not before.

```python
@pytest.fixture
async def clean_habits(test_engine):
    yield  # Test runs
    # Cleanup after - too late!
    async with AsyncSession(test_engine) as session:
        await session.execute(HabitLog.__table__.delete())
```

**Recommendation**: Clean before test:

```python
@pytest.fixture
async def clean_habits(test_engine):
    async with AsyncSession(test_engine) as session:
        await session.execute(HabitLog.__table__.delete())
        await session.execute(Habit.__table__.delete())
        await session.commit()
    yield  # Test runs with clean state
```

### 6.5 Factory Pattern for Test Data

**Recommendation**: Use `factory_boy` or similar:

```python
# fixtures/habits.py
import factory
from factory.alchemy import SQLAlchemyModelFactory

class HabitFactory(SQLAlchemyModelFactory):
    class Meta:
        model = Habit
        sqlalchemy_session_persistence = "commit"

    name = factory.Sequence(lambda n: f"Habit {n}")
    value_type = ValueType.boolean
    frequency = TargetFrequency.daily
    color = "#3B82F6"
    icon = "fas fa-check"

class HabitLogFactory(SQLAlchemyModelFactory):
    class Meta:
        model = HabitLog

    habit = factory.SubFactory(HabitFactory)
    log_date = factory.LazyFunction(date.today)
    value = Decimal("1")
```

### 6.6 Test Configuration

**Current Issue**: Tests use real database credentials.

**Recommendation**: Separate test configuration:

```python
# conftest.py
@pytest.fixture(scope="session")
def test_settings():
    return Settings(
        stage="test",
        postgres_db="gv_test",
        api_key="test-api-key-32-characters-long",
        # ...
    )
```

---

## 7. DevOps & Infrastructure

### 7.1 Docker Improvements

**Current Issues in `docker-compose.yml`**:

1. **Health check path incorrect** (line 37):
```yaml
test: ["CMD", "wget", "-q", "--spider", "http://localhost:8000/health"]
```
Should be:
```yaml
test: ["CMD", "wget", "-q", "--spider", "http://localhost:8000/api/v1/health"]
```

2. **Missing resource limits**:
```yaml
api:
  deploy:
    resources:
      limits:
        cpus: '0.5'
        memory: 512M
      reservations:
        cpus: '0.25'
        memory: 256M
```

3. **No restart policy**:
```yaml
api:
  restart: unless-stopped
```

### 7.2 Multi-Stage Dockerfile

**Recommendation**: Optimize image size:

```dockerfile
# Build stage
FROM python:3.14-slim AS builder
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN pip install uv && uv sync --frozen --no-dev

# Runtime stage
FROM python:3.14-slim AS runtime
WORKDIR /app
COPY --from=builder /app/.venv /app/.venv
COPY app/ app/
ENV PATH="/app/.venv/bin:$PATH"
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

### 7.3 Logging Improvements

**Current State**: Basic logging with `basicConfig`.

**Recommendation**: Structured JSON logging for production:

```python
import structlog

def setup_logging() -> None:
    settings = get_settings()

    if settings.is_dev:
        structlog.configure(
            processors=[structlog.dev.ConsoleRenderer()]
        )
    else:
        structlog.configure(
            processors=[
                structlog.processors.TimeStamper(fmt="iso"),
                structlog.processors.JSONRenderer()
            ]
        )

# Usage
logger = structlog.get_logger()
logger.info("habit_created", habit_id=123, name="Exercise")
```

### 7.4 Health Check Improvements

**Current State**: Simple DB connectivity check.

**Recommendation**: Comprehensive health endpoint:

```python
@router.get("/health")
async def health(conn: DbConnectionDep) -> dict:
    return {
        "status": "ok",
        "timestamp": datetime.now(TZ).isoformat(),
        "version": settings.app_version,
        "checks": {
            "database": await check_database(conn),
            "disk_space": check_disk_space(),
        }
    }

async def check_database(conn) -> dict:
    start = time.monotonic()
    await conn.execute(text("SELECT 1"))
    return {
        "status": "ok",
        "latency_ms": round((time.monotonic() - start) * 1000, 2)
    }
```

### 7.5 Metrics & Observability

**Recommendation**: Add Prometheus metrics:

```python
from prometheus_client import Counter, Histogram, generate_latest

REQUEST_COUNT = Counter(
    "http_requests_total",
    "Total HTTP requests",
    ["method", "endpoint", "status"]
)

REQUEST_LATENCY = Histogram(
    "http_request_duration_seconds",
    "HTTP request latency",
    ["endpoint"]
)

@app.middleware("http")
async def metrics_middleware(request: Request, call_next):
    start = time.monotonic()
    response = await call_next(request)
    duration = time.monotonic() - start

    REQUEST_COUNT.labels(
        method=request.method,
        endpoint=request.url.path,
        status=response.status_code
    ).inc()
    REQUEST_LATENCY.labels(endpoint=request.url.path).observe(duration)

    return response
```

### 7.6 CI/CD Pipeline

**Recommendation**: GitHub Actions workflow:

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - run: uv sync --dev
      - run: uv run ruff check .
      - run: uv run ruff format --check .

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:18-alpine
        env:
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
          POSTGRES_DB: gv_test
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - run: uv sync --dev
      - run: uv run pytest --cov=app --cov-report=xml
      - uses: codecov/codecov-action@v4
```

---

## 8. Business Logic Improvements

### 8.1 Soft Deletes

**Current State**: Hard deletes lose historical data.

**Recommendation**: Add soft delete support:

```python
class SoftDeleteMixin:
    deleted_at: Mapped[datetime | None] = mapped_column(default=None)

    @property
    def is_deleted(self) -> bool:
        return self.deleted_at is not None

class Habit(SoftDeleteMixin, Base):
    ...

# In repository
async def get_all(self, include_deleted: bool = False, ...):
    filters = []
    if not include_deleted:
        filters.append(self.model.deleted_at.is_(None))
    ...
```

### 8.2 Habit Categories/Tags

**Current State**: Habits have no categorization.

**Recommendation**: Add tagging system:

```python
class Tag(Base):
    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str] = mapped_column(String(50), unique=True)
    color: Mapped[str] = mapped_column(String(7))

class HabitTag(Base):
    habit_id: Mapped[int] = mapped_column(ForeignKey("habit.id"), primary_key=True)
    tag_id: Mapped[int] = mapped_column(ForeignKey("tag.id"), primary_key=True)
```

Benefits:
- Group related habits (Health, Work, Personal)
- Filter dashboard by category
- Analytics by category

### 8.3 Habit Templates

**Recommendation**: Pre-defined habit templates:

```python
class HabitTemplate(Base):
    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str]
    description: Mapped[str]
    value_type: Mapped[ValueType]
    suggested_target: Mapped[Decimal | None]
    category: Mapped[str]  # "health", "productivity", etc.

# Endpoint
@router.get("/templates")
async def list_templates(): ...

@router.post("/from-template/{template_id}")
async def create_from_template(template_id: int): ...
```

### 8.4 Reminder System

**Current State**: No reminder support.

**Recommendation**: Add reminder configuration:

```python
class HabitReminder(Base):
    habit_id: Mapped[int] = mapped_column(ForeignKey("habit.id"))
    reminder_time: Mapped[time]
    days_of_week: Mapped[list[int]]  # [0,1,2,3,4,5,6] for Mon-Sun
    notification_type: Mapped[str]  # "push", "email"
    is_enabled: Mapped[bool] = mapped_column(default=True)
```

### 8.5 Notes on Habit Logs

**Current State**: Logs only track value.

**Recommendation**: Add notes field:

```python
class HabitLog(Base):
    ...
    note: Mapped[str | None] = mapped_column(String(500), default=None)
```

Use cases:
- "Felt great after workout"
- "Skipped due to illness"
- Context for future analysis

### 8.6 Archiving Habits

**Recommendation**: Archive instead of delete:

```python
class Habit(Base):
    ...
    archived_at: Mapped[datetime | None] = mapped_column(default=None)

    @property
    def is_archived(self) -> bool:
        return self.archived_at is not None

@router.post("/{habit_id}/archive")
async def archive_habit(habit_id: int): ...

@router.post("/{habit_id}/unarchive")
async def unarchive_habit(habit_id: int): ...
```

### 8.7 Streak Protection / Grace Days

**Current State**: Missing one day breaks streak immediately.

**Recommendation**: Configurable grace period:

```python
class Habit(Base):
    ...
    grace_days: Mapped[int] = mapped_column(default=0)  # 0-2 days

# In streak calculation
def _calculate_streaks(self, habit: Habit, logs: list[HabitLog]):
    grace_remaining = habit.grace_days
    # Allow up to grace_days consecutive misses without breaking streak
    ...
```

---

## 9. API Design Improvements

### 9.1 Consistent Error Response Format

**Current State**: Errors return different structures.

**Recommendation**: Standardized error response:

```python
class ErrorResponse(BaseModel):
    error: str
    code: str
    details: dict | None = None
    request_id: str

# Example response
{
    "error": "Habit not found",
    "code": "HABIT_NOT_FOUND",
    "details": {"habit_id": 123},
    "request_id": "abc123"
}
```

### 9.2 Global Exception Handler

**Current State**: FastAPI default exception handling.

**Recommendation**: Custom exception handler:

```python
@app.exception_handler(AppException)
async def app_exception_handler(request: Request, exc: AppException):
    return JSONResponse(
        status_code=exc.status_code,
        content={
            "error": exc.detail,
            "code": exc.__class__.__name__.upper(),
            "request_id": request.state.request_id
        }
    )

@app.exception_handler(ValidationError)
async def validation_exception_handler(request: Request, exc: ValidationError):
    # Transform Pydantic errors to consistent format
    ...
```

### 9.3 API Versioning Strategy

**Current State**: Version in URL prefix (`/api/v1`).

**Recommendation**: Document versioning strategy:

```python
# Keep URL versioning but plan for changes
# /api/v1 - Current stable
# /api/v2 - Future breaking changes

# Add deprecation headers
@app.middleware("http")
async def add_deprecation_header(request, call_next):
    response = await call_next(request)
    if request.url.path.startswith("/api/v1/deprecated"):
        response.headers["Deprecation"] = "true"
        response.headers["Sunset"] = "2025-06-01"
    return response
```

### 9.4 OpenAPI Documentation Improvements

**Current State**: Basic auto-generated docs.

**Recommendation**: Enhanced documentation:

```python
app = FastAPI(
    title="Gestor de Vida API",
    description="""
    ## Habit Tracking API

    This API provides endpoints for managing habits and tracking daily progress.

    ### Features
    - Create and manage habits
    - Log daily habit completions
    - View statistics and streaks
    - Daily progress dashboard

    ### Authentication
    All endpoints require API key authentication via `X-API-Key` header.
    """,
    version="0.1.0",
    contact={
        "name": "Support",
        "email": "support@example.com"
    },
    license_info={
        "name": "MIT",
    }
)
```

### 9.5 Missing Endpoints

**Recommendations**:

```python
# Batch operations
@router.post("/batch")
async def batch_create_habits(habits: list[HabitCreate]) -> list[HabitRead]: ...

# Export data
@router.get("/export")
async def export_habits(format: Literal["json", "csv"] = "json"): ...

# Import data
@router.post("/import")
async def import_habits(file: UploadFile): ...

# Habit ordering/positioning
@router.patch("/reorder")
async def reorder_habits(order: list[int]): ...
```

### 9.6 Response Compression

**Recommendation**: Enable gzip compression:

```python
from fastapi.middleware.gzip import GZipMiddleware

app.add_middleware(GZipMiddleware, minimum_size=1000)
```

---

## 10. Database Improvements

### 10.1 Missing Indexes

**Current State**: Basic indexes on `habit_log` table.

**Recommendation**: Add compound indexes:

```python
class HabitLog(Base):
    __table_args__ = (
        UniqueConstraint("habit_id", "log_date", name="uq_habit_log_date"),
        Index("ix_habit_log_date_range", "habit_id", "log_date"),  # For date range queries
        Index("ix_habit_log_created", "created_at"),  # For sync operations
    )
```

### 10.2 Cascade Deletes

**Current State**: Manual cascade in application code.

**Recommendation**: Database-level cascade:

```python
class HabitLog(Base):
    habit_id: Mapped[int] = mapped_column(
        ForeignKey("habit.id", ondelete="CASCADE"),
        index=True
    )
```

### 10.3 Database Constraints

**Recommendation**: Add CHECK constraints:

```sql
-- In migration
ALTER TABLE habit ADD CONSTRAINT chk_target_range
    CHECK (target_min IS NULL OR target_max IS NULL OR target_min < target_max);

ALTER TABLE habit_log ADD CONSTRAINT chk_value_positive
    CHECK (value >= 0);
```

### 10.4 Audit Trail

**Recommendation**: Track changes:

```python
class AuditMixin:
    created_by: Mapped[str | None] = mapped_column(default=None)
    updated_by: Mapped[str | None] = mapped_column(default=None)

# Or dedicated audit table
class AuditLog(Base):
    id: Mapped[int] = mapped_column(primary_key=True)
    table_name: Mapped[str]
    record_id: Mapped[int]
    action: Mapped[str]  # INSERT, UPDATE, DELETE
    old_values: Mapped[dict | None]  # JSON
    new_values: Mapped[dict | None]  # JSON
    timestamp: Mapped[datetime]
```

### 10.5 Database Backups

**Recommendation**: Add backup script to Makefile:

```makefile
backup:
    docker compose exec db pg_dump -U $(POSTGRES_USER) $(POSTGRES_DB) > backup_$(date +%Y%m%d_%H%M%S).sql

restore:
    docker compose exec -T db psql -U $(POSTGRES_USER) $(POSTGRES_DB) < $(file)
```

---

## 11. Documentation Improvements

### 11.1 Missing Documentation

**Current State**:
- Basic API docs in `/docs/api.md`
- Habit domain docs in `/doc/habit.md`
- No deployment documentation

**Recommendations**:

```
docs/
├── api.md           # API reference (exists)
├── deployment.md    # Deployment guide
├── development.md   # Development setup
├── architecture.md  # System architecture
├── changelog.md     # Version history
└── contributing.md  # Contribution guidelines
```

### 11.2 Inline Code Documentation

**Current State**: Minimal docstrings.

**Recommendation**: Add module and class docstrings:

```python
"""
Habit Service Module

This module contains the business logic for habit management including:
- CRUD operations for habits
- Statistics calculation
- Streak tracking
- Daily progress computation
"""

class HabitService:
    """
    Service class for habit-related business logic.

    This service handles all habit operations and coordinates between
    repositories for data access. It implements the core business rules
    for habit tracking including:

    - Duplicate name prevention
    - Target validation
    - Streak calculation based on frequency
    - Completion rate computation

    Attributes:
        habit_repo: Repository for habit data access
        log_repo: Repository for habit log data access
    """
```

### 11.3 API Examples in OpenAPI

**Recommendation**: Add request/response examples:

```python
@router.post(
    "",
    responses={
        201: {
            "description": "Habit created successfully",
            "content": {
                "application/json": {
                    "example": {
                        "id": 1,
                        "name": "Exercise",
                        "value_type": "boolean",
                        "frequency": "daily",
                        "created_at": "2024-01-01T00:00:00Z"
                    }
                }
            }
        }
    }
)
async def create_habit(data: HabitCreate, ...): ...
```

---

## 12. Future Architecture Considerations

### 12.1 Multi-Tenancy

If expanding beyond personal use:

```python
class TenantMixin:
    tenant_id: Mapped[int] = mapped_column(ForeignKey("tenant.id"), index=True)

class Habit(TenantMixin, Base):
    ...

# Row-level security
async def get_all(self, tenant_id: int, ...):
    filters = [self.model.tenant_id == tenant_id]
    ...
```

### 12.2 Microservices Extraction

If the app grows significantly:

```
services/
├── habit-service/        # Habit CRUD & business logic
├── analytics-service/    # Statistics, streaks, reports
├── notification-service/ # Reminders, alerts
└── gateway/              # API Gateway, auth
```

### 12.3 Event Sourcing for Habits

For complete audit trail and time-travel:

```python
class HabitEvent(Base):
    id: Mapped[int]
    habit_id: Mapped[int]
    event_type: Mapped[str]  # "created", "updated", "logged", "completed"
    event_data: Mapped[dict]  # JSON payload
    timestamp: Mapped[datetime]

# Reconstruct state by replaying events
```

### 12.4 GraphQL API

For flexible frontend queries:

```python
import strawberry
from strawberry.fastapi import GraphQLRouter

@strawberry.type
class Habit:
    id: int
    name: str
    stats: "HabitStats" = strawberry.field(resolver=resolve_stats)
    logs: list["HabitLog"] = strawberry.field(resolver=resolve_logs)

@strawberry.type
class Query:
    habits: list[Habit]
    habit: Habit | None

graphql_app = GraphQLRouter(schema)
app.include_router(graphql_app, prefix="/graphql")
```

### 12.5 Mobile Sync Considerations

For offline-first mobile apps:

```python
class SyncMixin:
    sync_id: Mapped[UUID] = mapped_column(default_factory=uuid4)
    last_synced_at: Mapped[datetime | None]
    is_dirty: Mapped[bool] = mapped_column(default=False)

@router.post("/sync")
async def sync(
    client_changes: list[Change],
    last_sync: datetime
) -> SyncResponse:
    """
    Bidirectional sync endpoint for mobile clients.
    Returns server changes since last_sync and applies client_changes.
    """
    ...
```

---

## Summary of Priority Actions

### Immediate (Week 1-2)
1. Add comprehensive test coverage for habits CRUD
2. Fix health check path in docker-compose
3. Add habit_id validation in log update endpoint
4. Consolidate duplicate validation logic

### Short-term (Month 1)
1. Implement structured logging
2. Add rate limiting
3. Set up CI/CD pipeline
4. Add soft deletes
5. Improve error responses

### Medium-term (Month 2-3)
1. Add caching layer
2. Implement habit categories/tags
3. Add bulk operations
4. Database query optimization
5. Add metrics/observability

### Long-term (Quarter 2+)
1. Consider JWT authentication if multi-user
2. Add reminder system
3. Implement habit templates
4. GraphQL API for mobile flexibility
5. Event-driven architecture for notifications

---

*Document generated: 2025-12-13*
*Version: 1.0*
