FROM golang:alpine AS builder

WORKDIR /app
RUN apk add --no-cache musl-dev gcc ca-certificates
COPY . .
#RUN cd cmd/bws-cache && go build -race -ldflags='-s -w' -trimpath -o /dist/bws-cache
RUN cd cmd/bws-cache && go build -race -gcflags=all="-N -l" -o /dist/bws-cache 
#RUN ldd /dist/bws-cache | tr -s [:blank:] '\n' | grep ^/ | xargs -I % install -D % /dist/%

# Install Debugging env
#RUN go install -ldflags "-s -w -extldflags '-static'" github.com/go-delve/delve/cmd/dlv@latest


#FROM scratch
#COPY --from=builder /dist /
#COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Add debugger
#COPY --from=builder /go/bin/dlv /

USER 65534

CMD ["/dist/bws-cache", "start"]
#CMD ["dlv", "--listen=:4000", "--headless=true", "--api-version=2", "--log", "exec", "/dist/bws-cache", "start"]

