FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go tool templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -o build/wobsongo .
FROM alpine:3.21
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/build/wobsongo .
RUN addgroup -S wobsongo && adduser -S wobsongo -G wobsongo
RUN chown -R wobsongo:wobsongo /app
EXPOSE 8000
ENV PATH="$PATH:/app"
USER wobsongo
CMD ["wobsongo", "serve"]