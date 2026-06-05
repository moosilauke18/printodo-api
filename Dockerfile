FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build. admin_view.html is embedded into the binary via go:embed.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /printodo-api .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /printodo-api .

EXPOSE 8000

CMD ["./printodo-api"]
