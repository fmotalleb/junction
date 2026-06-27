FROM gcr.io/distroless/base-debian12:nonroot AS distroless

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/junction /usr/bin

ENTRYPOINT [ "/usr/bin/junction" ]
CMD [ "--help" ]
