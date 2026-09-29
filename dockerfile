FROM golang:1.22.5-bookworm

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y \
    gcc-mingw-w64-x86-64 \
    g++-mingw-w64-x86-64 \
    make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app