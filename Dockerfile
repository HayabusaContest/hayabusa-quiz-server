FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hayabusa-quiz-server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /hayabusa-quiz-server /app/hayabusa-quiz-server
COPY config /app/config
COPY data /app/data
EXPOSE 8080
ENTRYPOINT ["/app/hayabusa-quiz-server", "-c", "/app/config/config.yml"]
