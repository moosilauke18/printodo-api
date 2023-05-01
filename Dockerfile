FROM golang:1.11 as builder

WORKDIR /go/src/github.com/moosilauke18/printodo-api

COPY . .

RUN go get -d -v ./...

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/printodo-api .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /go/bin/printodo-api .

EXPOSE 8000

CMD ["./printodo-api"]
