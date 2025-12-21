FROM python:3.14-slim

ARG STAGE=prod
ENV STAGE=${STAGE}

WORKDIR /app

RUN pip install uv

COPY pyproject.toml .
RUN uv pip install --system -r pyproject.toml

COPY app/ ./app/

CMD if [ "$STAGE" = "dev" ]; then \
      uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload; \
    else \
      uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload; \
    fi
