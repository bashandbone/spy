# Dockerfile sample
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/spy ./cmd/spy

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/spy /usr/local/bin/spy

ENV PATH="/usr/local/bin:${PATH}"
EXPOSE 8080
USER nobody

ENTRYPOINT ["/usr/local/bin/spy"]
CMD ["--help"]
