# loadlore/synthetic: the configurable stand-in for an application service.
# Build from the repository root: make image
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/akinfemi/load-lore/internal/version.Version=${VERSION}" -o /out/synthetic ./cmd/synthetic

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/synthetic /synthetic
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/synthetic"]
