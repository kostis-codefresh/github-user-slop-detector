FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY pkg pkg
COPY ui ui
COPY cmd/contributor-report-web cmd/contributor-report-web
# .git is not in the build context, so the version comes in as a build arg.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=${VERSION}" -o /out/contributor-report-web ./cmd/contributor-report-web

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/contributor-report-web /contributor-report-web

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/contributor-report-web"]
