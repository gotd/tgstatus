FROM gcr.io/distroless/static

ADD tgstatd /usr/local/bin/tgstatd

ENTRYPOINT ["tgstatd"]
