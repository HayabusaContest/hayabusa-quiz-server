FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /arena-server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /arena-server /app/arena-server
COPY config.yml /app/config.yml
COPY data /app/data
EXPOSE 8080
ENTRYPOINT ["/app/arena-server", "-c", "/app/config.yml"]
