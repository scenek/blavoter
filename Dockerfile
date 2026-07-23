FROM golang:1.26.5-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:latest
WORKDIR /app
RUN addgroup -S blavoter && adduser -S -G blavoter blavoter
COPY --from=builder --chown=blavoter:blavoter /app/server .
COPY --chown=blavoter:blavoter static ./static
USER blavoter
EXPOSE 8080
CMD ["./server"]
