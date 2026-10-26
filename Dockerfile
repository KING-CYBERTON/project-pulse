# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/KING-CYBERTON/project-pulse/internal/server.Version=${VERSION}" -o /out/pulse ./cmd/pulse

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/pulse /app/pulse
COPY projects.json /app/projects.json
ENV PULSE_ADDR=:8080 PULSE_CONFIG=/app/projects.json
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/pulse"]
