# debian
FROM golang:1.25.3@sha256:6ea52a02734dd15e943286b048278da1e04eca196a564578d718c7720433dbbe AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y wget xz-utils && \
    wget https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz && \
    tar xf ffmpeg-release-amd64-static.tar.xz && \
    mv ffmpeg-*-static ffmpeg-static && \
    rm ffmpeg-release-amd64-static.tar.xz

RUN apt-get install -y pkg-config libavformat-dev libavcodec-dev libavutil-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 \
    CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)" \
    CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)" \
    go build -v -ldflags "-s -w" -o steam-query .

ENTRYPOINT ["/app/steam-query"]

# Run with:
# docker build -f Dockerfile -t steam-query .
# docker run -v ./output:/app/output steam-query -game-page https://store.steampowered.com/app/1063730/New_World_Aeternum/