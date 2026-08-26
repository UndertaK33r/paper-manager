FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o /out/paper-manager ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/paper-manager ./paper-manager
COPY web ./web
ENV PORT=8080
ENV DATA_DIR=/data
ENV UPLOAD_DIR=/data/uploads
ENV WEB_DIR=/app/web
VOLUME ["/data"]
EXPOSE 8080
CMD ["./paper-manager"]
