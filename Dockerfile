FROM golang:alpine AS builder

WORKDIR /app
RUN apk add --no-cache musl-dev gcc ca-certificates
COPY . .
RUN cd cmd/bws-cache && go build -ldflags='-s -w' -trimpath -o /dist/bws-cache
RUN ldd /dist/bws-cache | tr -s [:blank:] '\n' | grep ^/ | xargs -I % install -D % /dist/%

FROM scratch
COPY --from=builder /dist /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 65534

CMD ["/bws-cache", "start"]
