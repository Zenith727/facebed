FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o facebed .

FROM alpine:3.18
RUN apk --no-cache add ca-certificates
WORKDIR /facebed
COPY --from=builder /app/facebed /facebed/facebed
COPY --from=builder /app/assets /facebed/assets
COPY --from=builder /app/crawler-user-agents.json /facebed/crawler-user-agents.json

EXPOSE 9812
ENTRYPOINT ["/facebed/facebed"]
