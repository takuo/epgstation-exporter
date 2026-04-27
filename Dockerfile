FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /epgstation-exporter \
    ./cmd/epgstation-exporter

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /epgstation-exporter /epgstation-exporter

EXPOSE 9888

ENTRYPOINT ["/epgstation-exporter"]
