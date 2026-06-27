FROM golang:1.26.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/br-registry ./cmd/br-registry

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/br-registry /app/br-registry
COPY migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/br-registry"]
