FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /runner .
FROM golang:1.23-alpine
RUN apk add --no-cache git python3 ca-certificates build-base docker-cli && addgroup -S sentinel && adduser -S -G sentinel -h /home/sentinel sentinel
ENV HOME=/home/sentinel GOCACHE=/home/sentinel/.cache/go GOPATH=/home/sentinel/go
COPY --from=build /runner /usr/local/bin/runner
USER root
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s CMD runner healthcheck
ENTRYPOINT ["runner"]
