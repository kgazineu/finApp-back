# Estágios: base (dependências) → dev (air, hot reload) | build → imagem final.
# Sem --target, constrói a imagem de produção (último estágio), como no CI.
FROM golang:1.26.8-alpine3.23 AS base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# desenvolvimento: o código entra como volume (compose.yaml) e o air recompila a cada mudança
FROM base AS dev

ARG AIR_VERSION=v1.67.4
RUN go install github.com/air-verse/air@${AIR_VERSION}

ENV TZ=America/Sao_Paulo

EXPOSE 8080

# polling: eventos de arquivo nem sempre atravessam o volume do Docker Desktop
CMD ["air", "-c", ".air.toml", "-build.poll", "true"]

FROM base AS build

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

# "hoje", vencimentos e meses da projeção seguem o fuso do servidor (o binário embute o tzdata)
ENV TZ=America/Sao_Paulo

USER app

EXPOSE 8080

ENTRYPOINT ["/app/finapp-api"]
