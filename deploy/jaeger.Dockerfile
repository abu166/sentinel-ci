FROM golang:1.23-alpine AS health
WORKDIR /src
COPY trace-health.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /trace-health trace-health.go
FROM cr.jaegertracing.io/jaegertracing/jaeger:2.22.0
COPY --from=health /trace-health /trace-health
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s CMD ["/trace-health", "http://127.0.0.1:16686/api/v3/services"]
