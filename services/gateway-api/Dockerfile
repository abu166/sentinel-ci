FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /gateway .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
COPY --from=build /gateway /usr/local/bin/gateway
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["gateway"]
