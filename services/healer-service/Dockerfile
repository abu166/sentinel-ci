FROM golang:1.23-bookworm AS golang
FROM docker:27-cli AS dockercli
FROM python:3.12-slim-bookworm
COPY --from=golang /usr/local/go /usr/local/go
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
ENV PATH="/usr/local/go/bin:${PATH}" PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev git && rm -rf /var/lib/apt/lists/*
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && useradd --create-home sentinel
COPY healer ./healer
USER sentinel
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=5s CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8081/api/v1/health')"
CMD ["uvicorn", "healer.app:app", "--host", "0.0.0.0", "--port", "8081"]
