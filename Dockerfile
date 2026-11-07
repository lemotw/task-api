# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/task-api ./cmd/server \
 && mkdir -p /out/data

# distroless has no shell; /data is created in the build stage and handed to the
# nonroot user so a mounted volume is writable (PER-29).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/task-api /task-api
COPY --from=build --chown=nonroot:nonroot /out/data /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/task-api"]
