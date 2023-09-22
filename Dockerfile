FROM golang:1.21-alpine AS build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN apk update && apk add --no-cache make
RUN make

FROM alpine:latest

COPY --from=build /build/bin/go-telexpenses /usr/bin
COPY --from=build /build/migrations /root/migrations

RUN apk add --no-cache
EXPOSE 8080
ENTRYPOINT ["go-telexpenses"]
