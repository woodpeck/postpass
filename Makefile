postpass-server: cmd/postpass/main.go postpass/*.go
	go build -o postpass-server ./cmd/postpass
