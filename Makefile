build:
	CO_ENABLED=1 go build -ldflags "-s -w -linkmode external -extldflags -static" .
