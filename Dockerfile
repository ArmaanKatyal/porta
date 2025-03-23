# ======================
#       BUILDER STAGE
# ======================
FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o ./bin/server ./cmd

# ======================
#     RUNNER STAGE
# ======================
FROM alpine:latest

COPY --from=builder /app/bin/server /server

EXPOSE 8080

CMD ["/server"]
