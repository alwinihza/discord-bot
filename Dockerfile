FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bot .

FROM alpine:3.22
RUN apk add --no-cache sqlite && adduser -D -H bot && mkdir /data && chown bot /data
COPY --from=builder /bot /usr/local/bin/bot
USER bot
VOLUME /data
CMD ["bot"]
