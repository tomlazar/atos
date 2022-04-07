# setup builder image
FROM golang:1.16-buster as builder 
WORKDIR /atos

# copy and download mod file
COPY go.mod .
COPY go.sum .
RUN go mod download

# build app
COPY ./*.go ./
RUN go build -o ./bin/atos .

# Now copy it into our base image.
FROM gcr.io/distroless/base
COPY --from=builder /atos/bin/atos /atos
CMD ["/atos"]