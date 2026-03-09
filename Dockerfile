FROM gcr.io/distroless/base-debian12:nonroot AS distroless

ARG TARGETPLATFORM=""
COPY $TARGETPLATFORM/junction /usr/bin

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


FROM library/debian:trixie-slim AS standalone

COPY /docker/root/ /
COPY --from=builder /app/junction /usr/bin/junction

ENV HTTP_PORT=80 \
  SNI_PORT=443 \
  UDP_BUFFER=65507 

ENTRYPOINT [ "/docker-entrypoint.sh" ]
CMD [ "junction", "--config=/etc/junction/config.toml" ]