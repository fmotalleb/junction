FROM library/debian:bookworm-slim AS slim

COPY /docker/root/ /
COPY junction /usr/bin/junction


ENV HTTP_PORT=80 \
  SNI_PORT=443 \
  VLESS_PACKET_ENCODING= \
  VLESS_ADDRESS= \
  VLESS_PORT= \
  VLESS_UUID= \
  VLESS_TLS_ENABLED= \
  VLESS_TLS_INSECURE= \
  VLESS_SNI= \
  VLESS_UTLS_ENABLED= \
  VLESS_UTLS_FINGERPRINT= \
  VLESS_TRANSPORT_PATH= \
  VLESS_TRANSPORT_TYPE= \
  VLESS_HOSTNAME_HEADER=


ENTRYPOINT [ "/usr/bin/junction" ]
CMD [ "--config=/etc/junction/config.toml" ]

FROM gcr.io/distroless/base-debian12:nonroot AS distroless

COPY junction /usr/bin/junction

ENTRYPOINT [ "/usr/bin/junction" ]
CMD [ "--help" ]


FROM golang:latest AS builder
RUN mkdir /app
COPY go.mod /app/
COPY go.sum /app/
WORKDIR /app
RUN go mod download
COPY ./ /app
RUN CGO_ENABLED=0 go build -o junction
RUN chmod +x junction


FROM library/debian:bookworm-slim AS standalone

COPY /docker/root/ /
COPY --from=builder /app/junction /usr/bin/junction

ENV HTTP_PORT=80 \
  SNI_PORT=443 \
  VLESS_PACKET_ENCODING= \
  VLESS_ADDRESS= \
  VLESS_PORT= \
  VLESS_UUID= \
  VLESS_TLS_ENABLED= \
  VLESS_TLS_INSECURE= \
  VLESS_SNI= \
  VLESS_UTLS_ENABLED= \
  VLESS_UTLS_FINGERPRINT= \
  VLESS_TRANSPORT_PATH= \
  VLESS_TRANSPORT_TYPE= \
  VLESS_HOSTNAME_HEADER=


ENTRYPOINT [ "/usr/bin/junction" ]
CMD [ "--config=/etc/junction/config.toml" ]