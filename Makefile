build:
	CGO_ENABLED=1 \
    CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)" \
    CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)" \
    go build -v -ldflags "-s -w" -o steam-query .
