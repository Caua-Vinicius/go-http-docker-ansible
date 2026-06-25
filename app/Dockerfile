FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN adduser -D -g '' appuser

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o http-server-projeto-korp .

FROM scratch

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /app/http-server-projeto-korp /http-server-projeto-korp

USER appuser

EXPOSE 8080

ENTRYPOINT ["/http-server-projeto-korp"]