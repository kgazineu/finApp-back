FROM golang:1.26.8-alpine3.23 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/finapp-api \
    ./cmd/api

FROM alpine:3.23.5

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=build /out/finapp-api ./finapp-api

USER app

EXPOSE 8080

ENTRYPOINT ["/app/finapp-api"]
