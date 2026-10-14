FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ui ui
COPY cmd/contributor-report-web cmd/contributor-report-web
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/contributor-report-web ./cmd/contributor-report-web

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/contributor-report-web /contributor-report-web

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/contributor-report-web"]
