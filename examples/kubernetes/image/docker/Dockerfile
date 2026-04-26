ARG PGVERSION

# base build image
FROM golang:1.16-buster AS build_base

WORKDIR /orion

# only copy go.mod and go.sum
COPY go.mod .
COPY go.sum .

RUN go mod download

#######
####### Build the orion binaries
#######
FROM build_base AS builder

# copy all the sources
COPY . .

RUN make

#######
####### Build the final image
#######
FROM postgres:$PGVERSION

RUN useradd -ms /bin/bash orion

EXPOSE 5432

# copy the binary
COPY --from=builder /orion/bin/ /usr/local/bin

RUN chmod +x /usr/local/bin/orion-keeper /usr/local/bin/orion-sentinel /usr/local/bin/orion-proxy /usr/local/bin/orionctl
