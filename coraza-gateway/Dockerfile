# Build the Coraza gateway as a self-contained, non-root image.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/coraza-gateway .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/coraza-gateway /usr/local/bin/coraza-gateway
COPY --from=busybox:1.36.1-musl /bin/wget /busybox/wget
USER nonroot:nonroot
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/coraza-gateway"]
