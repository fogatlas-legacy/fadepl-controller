# Setup GO builder container
FROM golang as builder

WORKDIR /go/src/gihub.com/fogatlas/fadepl-controller

COPY ./ .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app main.go

# Build application container from GO artifact
FROM scratch
WORKDIR /

COPY --from=builder /go/src/gihub.com/fogatlas/fadepl-controller/app /bin/fadepl-controller

ENTRYPOINT ["fadepl-controller"]
