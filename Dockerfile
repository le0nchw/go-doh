FROM golang:alpine as builder
WORKDIR /app

COPY . .
RUN go mod download && go mod verify \
&& sh build.sh



FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/doh /app/doh

CMD ["/app/doh"]