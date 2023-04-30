FROM golang:1.20
RUN GOOS=linux GARCH=amd64 CGO_ENABLED=0 go install -v -a -installsuffix cgo github.com/trazfr/prometheus-waze-exporter@latest

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=0 /go/bin/prometheus-waze-exporter ./
ENTRYPOINT ["/root/prometheus-waze-exporter"]
CMD ["/config/config.json"]
