FROM golang:1.25-alpine AS builder
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X github.com/nightshiftco/bots/internal/version.Version=${VERSION}" \
    -o /out/nightshift-slack-bot ./cmd/nightshift-slack-bot

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/nightshift-slack-bot /usr/local/bin/nightshift-slack-bot
COPY skills/ /etc/skills/
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/nightshift-slack-bot"]
