# syntax=docker/dockerfile:1

# --- builder: install dependencies into a venv -------------------------
FROM python:3.12-alpine AS builder
WORKDIR /build
COPY requirements.txt .
RUN python -m venv /venv \
    && /venv/bin/pip install --no-cache-dir --upgrade pip \
    && /venv/bin/pip install --no-cache-dir -r requirements.txt

# --- runtime: nothing but the venv + app code ---------------------------
FROM python:3.12-alpine AS runtime

# Alpine's own package manager isn't needed at all at runtime; pure-python
# deps mean no gcc/musl-dev has to be installed here either.
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /venv /venv
COPY app ./app
COPY static ./static
COPY scripts ./scripts

ENV PATH="/venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DATA_DIR=/app/data \
    STATIC_DIR=/app/static \
    PORT=8000

# The data dir holds the sqlite db and is the only thing this container
# needs to write to; dockerRun.sh mounts it as a volume and the container
# itself runs with a read-only root filesystem.
RUN mkdir -p /app/data && chown -R app:app /app

USER app
EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -q -O- http://127.0.0.1:8000/ >/dev/null || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
