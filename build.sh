env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" cmd/doh.go

chmod +x doh