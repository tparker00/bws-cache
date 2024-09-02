FROM golang:alpine AS builder

RUN apk add --no-cache musl-dev gcc git
COPY . .

RUN go generate ./internal/pkg/config/config.go
RUN cd cmd/bws-cache && go build -ldflags='-s -w' --trimpath -o /dist/bws-cache 
RUN ldd /dist/bws-cache | tr -s [:blank:] '\n' | grep ^/ | xargs -I % install -D % /dist/%


FROM alpine
COPY --from=builder /dist /
RUN apk add --no-cache ca-certificates

USER 65534

CMD ["/bws-cache", "start"]

