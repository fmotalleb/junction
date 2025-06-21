FROM library/debian:bookworm-slim AS slim

RUN --mount=type=cache,target=/var/lib/apt/lists/ \
  --mount=type=cache,target=/var/cache/apt/archives/ \
  apt-get update \
  && apt-get full-upgrade -y \
  && apt-get install -y supervisor gettext

ARG SINGBOX_VERSION=1.11.9
ARG SINGBOX_ARCH=amd64
ARG SINGBOX_BIN_NAME=sing-box_${SINGBOX_VERSION}_linux_${SINGBOX_ARCH}.deb
ARG SINGBOX_URL=https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/${SINGBOX_BIN_NAME}
ADD ${SINGBOX_URL} /singbox/

RUN dpkg -i /singbox/${SINGBOX_BIN_NAME} \
  && rm -rf /singbox \
  && mkdir /etc/singbox/

COPY /docker/root/ /
COPY junction /usr/bin/junction

ENV VLESS_PROXY= \
  HTTP_PORT=80 \
  SNI_PORT=443

ENTRYPOINT [ "/docker-entrypoint.sh" ]
CMD [ "supervisord" ]

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

RUN --mount=type=cache,target=/var/lib/apt/lists/ \
  --mount=type=cache,target=/var/cache/apt/archives/ \
  apt-get update \
  && apt-get full-upgrade -y \
  && apt-get install -y supervisor gettext

ARG SINGBOX_VERSION=1.11.9
ARG SINGBOX_ARCH=amd64
ARG SINGBOX_BIN_NAME=sing-box_${SINGBOX_VERSION}_linux_${SINGBOX_ARCH}.deb
ARG SINGBOX_URL=https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/${SINGBOX_BIN_NAME}
ADD ${SINGBOX_URL} /singbox/

RUN dpkg -i /singbox/${SINGBOX_BIN_NAME} \
  && rm -rf /singbox \
  && mkdir /etc/singbox/

COPY /docker/root/ /
COPY --from=builder /app/junction /usr/bin/junction

ENV VLESS_PROXY= \
  HTTP_PORT=80 \
  SNI_PORT=443

ENTRYPOINT [ "/docker-entrypoint.sh" ]
CMD [ "supervisord" ]