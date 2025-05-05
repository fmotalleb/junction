ARG NGINX_TAG=stable-bookworm
FROM library/nginx:$NGINX_TAG

RUN --mount=type=cache,target=/var/lib/apt/lists/ \
  --mount=type=cache,target=/var/cache/apt/archives/ \
  apt-get update \
  && apt-get full-upgrade -y \
  && apt-get install -y supervisor

ARG SINGBOX_VERSION=1.11.9
ARG SINGBOX_ARCH=amd64
ARG SINGBOX_BIN_NAME=sing-box_${SINGBOX_VERSION}_linux_${SINGBOX_ARCH}.deb
ARG SINGBOX_URL=https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/${SINGBOX_BIN_NAME}
ADD ${SINGBOX_URL} /singbox/

RUN dpkg -i /singbox/${SINGBOX_BIN_NAME} \
  && rm -rf /singbox \
  && mkdir /etc/singbox/ \
  && mv /docker-entrypoint.sh /nginx-entrypoint.sh

COPY ./root/ /

ENV VLESS_PROXY=

CMD [ "supervisord" ]